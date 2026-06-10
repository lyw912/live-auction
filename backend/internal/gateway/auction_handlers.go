package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/singleflight"

	aicap "live-auction/backend/internal/ai"
	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/demo"
	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/realtime"
	"live-auction/backend/internal/redisengine"
	"live-auction/backend/internal/redisx"
	"live-auction/backend/internal/storage"
	apptracing "live-auction/backend/internal/tracing"
)

var auctionHTTPSnapshotLoadGroup singleflight.Group

type AuctionHandler struct {
	Config config.Config
	Deps   *storage.Dependencies
	Repo   *auction.Repository
	RT     *realtime.Server
	ACL    roomACL
	Bids   *bidAdmission
	Lanes  *bidLaneManager
	Guard  *redisGuard
	Engine *redisengine.Engine
	AIRepo *aicap.Repository
	AIGen  aicap.Generator
}

type uploadURLRequest struct {
	ObjectName  string `json:"object_name"`
	ContentType string `json:"content_type"`
}

type currentUserAuctionSnapshot struct {
	auction.Auction
	MaxBidIntent *auction.MaxBidIntent `json:"max_bid_intent,omitempty"`
}

const (
	// HTTP polling is a follower/read path. A short public cache absorbs
	// live-room refresh storms without making bid decisions depend on cached
	// state or hiding updates for long.
	auctionHTTPSnapshotCacheTTL     = 250 * time.Millisecond
	maxBidIntentAbsentCacheTTL      = 5 * time.Second
	maxBidIntentAbsentCacheSentinel = "1"
)

type demoCompetingBidRequest struct {
	BidderID      string `json:"bidder_id"`
	ClientBidID   string `json:"client_bid_id"`
	AmountCents   int64  `json:"amount_cents"`
	ClientSeenSeq int64  `json:"client_seen_seq"`
	Mode          string `json:"mode"`
}

type demoShortenEndRequest struct {
	RemainingSeconds int `json:"remaining_seconds"`
}

type liveOpsTaskRequest struct {
	TaskKey string `json:"task_key"`
}

type liveOpsTeamRequest struct {
	TeamKey string `json:"team_key"`
}

type liveOpsRewardConfigRequest struct {
	Enabled           bool   `json:"enabled"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	RewardName        string `json:"reward_name"`
	RewardQuota       int    `json:"reward_quota"`
	RequiredTaskCount int    `json:"required_task_count"`
}

type roomSummary struct {
	ID     string `json:"id"`
	HostID string `json:"host_id"`
	Status string `json:"status"`
	Role   string `json:"role"`
}

func (h AuctionHandler) ListRooms(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var rows pgx.Rows
	var err error
	if user.Role == "host" {
		rows, err = h.Deps.Postgres.Query(r.Context(), `
			SELECT id, host_id, status, 'host' AS role
			FROM rooms
			WHERE host_id = $1 AND status = 'OPEN'
			ORDER BY created_at DESC, id
		`, user.ID)
	} else {
		rows, err = h.Deps.Postgres.Query(r.Context(), `
			SELECT r.id, r.host_id, r.status, rm.role
			FROM room_memberships rm
			JOIN rooms r ON r.id = rm.room_id
			WHERE rm.user_id = $1
			  AND rm.status = 'ACTIVE'
			  AND rm.role IN ('viewer','host')
			  AND r.status = 'OPEN'
			ORDER BY rm.joined_at DESC, r.id
		`, user.ID)
	}
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	defer rows.Close()
	result := []roomSummary{}
	for rows.Next() {
		var room roomSummary
		if err := rows.Scan(&room.ID, &room.HostID, &room.Status, &room.Role); err != nil {
			writeResult(w, r, http.StatusOK, nil, err)
			return
		}
		result = append(result, room)
	}
	if err := rows.Err(); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h AuctionHandler) GetLiveOpsCampaign(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	campaign, err := h.Repo.GetLiveOpsCampaign(r.Context(), roomID, user.ID)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, campaign)
}

func (h AuctionHandler) GetHostLiveOpsSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireHostOwnsRoom(r.Context(), user, roomID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	summary, err := h.Repo.GetLiveOpsHostSummary(r.Context(), roomID)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h AuctionHandler) UpdateHostLiveOpsReward(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireHostOwnsRoom(r.Context(), user, roomID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	var req liveOpsRewardConfigRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	summary, err := h.Repo.UpdateLiveOpsRewardConfig(r.Context(), roomID, auction.LiveOpsRewardConfig{
		Enabled:           req.Enabled,
		Title:             req.Title,
		Description:       req.Description,
		RewardName:        req.RewardName,
		RewardQuota:       req.RewardQuota,
		RequiredTaskCount: req.RequiredTaskCount,
	})
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h AuctionHandler) CompleteLiveOpsTask(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	taskKey := chi.URLParam(r, "task_key")
	if taskKey == "" {
		var req liveOpsTaskRequest
		if err := decodeJSON(r, &req); err == nil {
			taskKey = req.TaskKey
		}
	}
	campaign, err := h.Repo.CompleteLiveOpsTask(r.Context(), roomID, user.ID, taskKey)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, campaign)
}

func (h AuctionHandler) SelectLiveOpsTeam(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	var req liveOpsTeamRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	campaign, err := h.Repo.SelectLiveOpsTeam(r.Context(), roomID, user.ID, req.TeamKey)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, campaign)
}

func (h AuctionHandler) EnterLiveOpsLuckyDraw(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	campaign, err := h.Repo.EnterLiveOpsLuckyDraw(r.Context(), roomID, user.ID)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, campaign)
}

func (h AuctionHandler) OpenLiveOpsLuckyDraw(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	campaign, err := h.Repo.OpenLiveOpsLuckyDraw(r.Context(), roomID, user.ID)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, campaign)
}

func (h AuctionHandler) CreateUploadURL(w http.ResponseWriter, r *http.Request) {
	var req uploadURLRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	if req.ObjectName == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "object_name is required", 400))
		return
	}
	url, err := h.Deps.MinIO.PresignedPutObject(context.Background(), h.Deps.Bucket, req.ObjectName, 15*time.Minute)
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "failed to create upload url", 500))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bucket":       h.Deps.Bucket,
		"object_name":  req.ObjectName,
		"upload_url":   url.String(),
		"public_url":   publicObjectURL(h.Config.S3UseSSL, h.Config.MinIOEndpoint, h.Deps.Bucket, req.ObjectName),
		"expires_in_s": 900,
	})
}

func publicObjectURL(useSSL bool, endpoint string, bucket string, objectName string) string {
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	return scheme + "://" + endpoint + "/" + bucket + "/" + objectName
}

func publicMediaURL(objectName string) string {
	return "/api/media/" + strings.TrimLeft(objectName, "/")
}

func (h AuctionHandler) UploadItemImage(w http.ResponseWriter, r *http.Request) {
	if h.Deps == nil || h.Deps.MinIO == nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "image storage is not available", http.StatusInternalServerError))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "image file is too large or invalid", http.StatusBadRequest))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "file is required", http.StatusBadRequest))
		return
	}
	defer file.Close()
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !strings.HasPrefix(contentType, "image/") {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "only image uploads are supported", http.StatusBadRequest))
		return
	}
	exts, _ := mime.ExtensionsByType(contentType)
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" && len(exts) > 0 {
		ext = exts[0]
	}
	if ext == "" {
		ext = ".bin"
	}
	objectName := fmt.Sprintf("items/%d-%s%s", time.Now().UnixNano(), sanitizeObjectStem(header.Filename), ext)
	info, err := h.Deps.MinIO.PutObject(r.Context(), h.Deps.Bucket, objectName, file, header.Size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "image upload failed", http.StatusInternalServerError))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"bucket":      h.Deps.Bucket,
		"object_name": objectName,
		"size":        info.Size,
		"public_url":  publicMediaURL(objectName),
	})
}

func (h AuctionHandler) ServeMedia(w http.ResponseWriter, r *http.Request) {
	if h.Deps == nil || h.Deps.MinIO == nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "image storage is not available", http.StatusInternalServerError))
		return
	}
	objectName := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if objectName == "" || strings.Contains(objectName, "..") || !strings.HasPrefix(objectName, "items/") {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid media path", http.StatusBadRequest))
		return
	}
	object, err := h.Deps.MinIO.GetObject(r.Context(), h.Deps.Bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "media not found", http.StatusNotFound))
		return
	}
	defer object.Close()
	info, err := object.Stat()
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "media not found", http.StatusNotFound))
		return
	}
	if info.ContentType != "" {
		w.Header().Set("Content-Type", info.ContentType)
	} else if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(objectName))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	if _, err := io.Copy(w, object); err != nil {
		return
	}
}

func sanitizeObjectStem(filename string) string {
	stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	stem = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, stem)
	stem = strings.Trim(stem, "-_")
	if stem == "" {
		return "upload"
	}
	if len(stem) > 60 {
		return stem[:60]
	}
	return stem
}

func (h AuctionHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var req auction.CreateItemInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	item, err := h.Repo.CreateItem(r.Context(), req)
	writeResult(w, r, http.StatusCreated, item, err)
}

func (h AuctionHandler) CreateAuction(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.CreateAuctionInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	if err := h.ACL.requireHostOwnsRoom(r.Context(), user, req.RoomID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.CreateAuction(r.Context(), req, traceID(r.Context()))
	writeResult(w, r, http.StatusCreated, result, err)
}

func (h AuctionHandler) UpdateRules(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.UpdateRulesInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.UpdateRules(r.Context(), auctionID, req, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Schedule(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req struct {
		StartAt *time.Time `json:"start_at"`
	}
	if r.Body != http.NoBody {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.Schedule(r.Context(), auctionID, req.StartAt, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Unschedule(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.Unschedule(r.Context(), auctionID, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Start(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.Start(r.Context(), auctionID, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.CancelInput
	if r.Body != http.NoBody {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.Cancel(r.Context(), auctionID, req, traceID(r.Context()))
	if err == nil && h.Engine != nil {
		// Best-effort fence: PG is the authoritative cancellation; this call
		// prevents the Redis hot engine from accepting bids after the PG cancel.
		// The reconciler's checkTerminalFenced check will detect and repair any
		// race between this fence and the PG write (e.g. Redis unreachable here).
		h.Engine.FenceAuction(r.Context(), auctionID, "HOST_CANCELLED")
	}
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) NarrateStart(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.NarrateStart(r.Context(), auctionID, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) NarrateStop(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.NarrateStop(r.Context(), auctionID, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ListAuctions(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := r.URL.Query().Get("room_id")
	if roomID != "" {
		if err := h.requireRoomListAccess(r, user, roomID); err != nil {
			writeResult(w, r, http.StatusOK, nil, err)
			return
		}
		result, err := h.Repo.ListAuctions(r.Context(), roomID)
		writeResult(w, r, http.StatusOK, result, err)
		return
	}
	roomIDs, err := h.ACL.accessibleRoomIDs(r.Context(), user)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.ListAuctionsForRooms(r.Context(), roomIDs)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ListRoomAuctions(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.requireRoomListAccess(r, user, roomID); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.ListAuctions(r.Context(), roomID)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) GetAuction(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.getAuctionForHTTPSnapshot(r.Context(), auctionID)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	snapshot := currentUserAuctionSnapshot{Auction: result}
	intent, ok, err := h.getMaxBidIntentForHTTPSnapshot(r.Context(), auctionID, user.ID)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	if ok {
		snapshot.MaxBidIntent = &intent
	}
	writeResult(w, r, http.StatusOK, snapshot, nil)
}

func (h AuctionHandler) getAuctionForHTTPSnapshot(ctx context.Context, auctionID string) (auction.Auction, error) {
	if h.Deps != nil && h.Deps.Redis != nil {
		if payload, err := h.Deps.Redis.Get(ctx, auctionHTTPSnapshotCacheKey(auctionID)).Bytes(); err == nil {
			var cached auction.Auction
			if err := json.Unmarshal(payload, &cached); err == nil && cached.ID == auctionID {
				cached.ServerTimeMS = currentUnixMillis()
				observability.Inc("auction_http_snapshot_cache_total", map[string]string{"result": "hit"})
				return cached, nil
			}
			observability.Inc("auction_http_snapshot_cache_total", map[string]string{"result": "invalid"})
		}
	}
	observability.Inc("auction_http_snapshot_cache_total", map[string]string{"result": "miss"})
	loadKey := fmt.Sprintf("%p:%s", h.Repo, auctionID)
	value, err, _ := auctionHTTPSnapshotLoadGroup.Do(loadKey, func() (any, error) {
		result, err := h.Repo.GetAuction(ctx, auctionID)
		if err != nil {
			return auction.Auction{}, err
		}
		if h.Deps != nil && h.Deps.Redis != nil {
			if payload, err := json.Marshal(result); err == nil {
				_ = h.Deps.Redis.Set(ctx, auctionHTTPSnapshotCacheKey(auctionID), payload, auctionHTTPSnapshotCacheTTL).Err()
			}
		}
		return result, nil
	})
	if err != nil {
		return auction.Auction{}, err
	}
	result, ok := value.(auction.Auction)
	if !ok {
		return auction.Auction{}, errors.New("auction http snapshot singleflight returned unexpected type")
	}
	return result, nil
}

func (h AuctionHandler) getMaxBidIntentForHTTPSnapshot(ctx context.Context, auctionID string, userID string) (auction.MaxBidIntent, bool, error) {
	if h.Deps != nil && h.Deps.Redis != nil {
		if cached, err := h.Deps.Redis.Get(ctx, maxBidIntentAbsentCacheKey(auctionID, userID)).Result(); err == nil && cached == maxBidIntentAbsentCacheSentinel {
			observability.Inc("auction_max_bid_intent_absent_cache_total", map[string]string{"result": "hit"})
			return auction.MaxBidIntent{}, false, nil
		}
	}
	observability.Inc("auction_max_bid_intent_absent_cache_total", map[string]string{"result": "miss"})
	intent, err := h.Repo.GetMaxBidIntent(ctx, auctionID, userID)
	if err != nil {
		if hasAPIErrorCode(err, apierrors.CodeAuctionNotFound) {
			if h.Deps != nil && h.Deps.Redis != nil {
				_ = h.Deps.Redis.Set(ctx, maxBidIntentAbsentCacheKey(auctionID, userID), maxBidIntentAbsentCacheSentinel, maxBidIntentAbsentCacheTTL).Err()
			}
			return auction.MaxBidIntent{}, false, nil
		}
		return auction.MaxBidIntent{}, false, err
	}
	return intent, true, nil
}

func (h AuctionHandler) invalidateMaxBidIntentSnapshotCache(ctx context.Context, auctionID string, userID string) {
	if h.Deps == nil || h.Deps.Redis == nil {
		return
	}
	_ = h.Deps.Redis.Del(ctx, maxBidIntentAbsentCacheKey(auctionID, userID)).Err()
}

func (h AuctionHandler) PlaceBid(w http.ResponseWriter, r *http.Request) {
	mode := h.Config.BidEngineMode
	if mode == "" {
		mode = bidEngineModeRedisLedger
	}
	ctx, span := apptracing.Start(r.Context(), "bid.place",
		attribute.String("bid.engine_mode", mode),
		attribute.String("http.route", "/api/auctions/{id}/bids"),
	)
	var handlerErr error
	defer func() {
		apptracing.End(span, handlerErr)
	}()
	r = r.WithContext(ctx)
	traceCtx := r.Context()
	totalStart := bidRequestStart(r.Context())
	totalOutcome := "error"
	defer func() {
		recordBidGatewayStage("total", mode, totalOutcome, time.Since(totalStart))
	}()

	stageStart := time.Now()
	_, stageSpan := apptracing.Start(traceCtx, "bid.auth")
	user, ok := currentUser(r)
	if !ok {
		apiErr := apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized)
		handlerErr = apiErr
		apptracing.End(stageSpan, handlerErr)
		recordBidGatewayStage("auth", mode, "error", time.Since(stageStart))
		writeError(w, r, apiErr)
		return
	}
	stageSpan.SetAttributes(attribute.String("enduser.id", user.ID), attribute.String("enduser.role", user.Role))
	apptracing.End(stageSpan, nil)
	recordBidGatewayStage("auth", mode, "ok", time.Since(stageStart))

	stageStart = time.Now()
	_, stageSpan = apptracing.Start(traceCtx, "bid.decode")
	var req auction.BidInput
	if err := decodeJSON(r, &req); err != nil {
		apiErr := apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400)
		handlerErr = apiErr
		apptracing.End(stageSpan, handlerErr)
		recordBidGatewayStage("decode", mode, "error", time.Since(stageStart))
		writeError(w, r, apiErr)
		return
	}
	stageSpan.SetAttributes(attribute.Int64("bid.amount_cents", req.AmountCents))
	apptracing.End(stageSpan, nil)
	recordBidGatewayStage("decode", mode, "ok", time.Since(stageStart))
	auctionID := chi.URLParam(r, "id")
	span.SetAttributes(attribute.String("auction.id", auctionID))

	if h.Engine == nil {
		err := apierrors.New(apierrors.CodeEnginePaused, "redis/kafka hot engine is required", http.StatusServiceUnavailable)
		handlerErr = err
		writeBidAdmissionResult(w, r, auction.BidResponse{}, err)
		return
	}
	if h.Bids != nil {
		stageStart = time.Now()
		_, stageSpan = apptracing.Start(traceCtx, "bid.admission", attribute.String("auction.id", auctionID))
		if replay, permit, ok, err := h.Bids.admit(r.Context(), r, user, auctionID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context())); err != nil || ok {
			if err != nil {
				handlerErr = err
				apptracing.End(stageSpan, err)
				recordBidGatewayStage("admission", mode, "error", time.Since(stageStart))
			} else if ok {
				apptracing.End(stageSpan, nil)
				recordBidGatewayStage("admission", mode, "replay", time.Since(stageStart))
				totalOutcome = "ok"
			}
			writeBidAdmissionResult(w, r, replay, err)
			return
		} else if permit != nil {
			apptracing.End(stageSpan, nil)
			recordBidGatewayStage("admission", mode, "ok", time.Since(stageStart))
			defer permit.Release()
		}
	}
	stageStart = time.Now()
	_, stageSpan = apptracing.Start(traceCtx, "bid.acl", attribute.String("auction.id", auctionID))
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		handlerErr = err
		apptracing.End(stageSpan, err)
		recordBidGatewayStage("acl", mode, "error", time.Since(stageStart))
		writeBidAdmissionResult(w, r, auction.BidResponse{}, err)
		return
	}
	apptracing.End(stageSpan, nil)
	recordBidGatewayStage("acl", mode, "ok", time.Since(stageStart))

	stageStart = time.Now()
	_, stageSpan = apptracing.Start(traceCtx, "bid.redis_engine", attribute.String("auction.id", auctionID))
	result, err := h.Engine.PlaceBid(r.Context(), auctionID, user.ID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context()),
		redisx.ACLMembershipKey(auctionID, user.ID))
	if err != nil {
		handlerErr = err
		apptracing.End(stageSpan, err)
		recordBidGatewayStage("redis_engine", mode, "error", time.Since(stageStart))
	} else {
		stageSpan.SetAttributes(
			attribute.String("bid.result", result.Result),
			attribute.Int64("bid.seq", result.Seq),
		)
		apptracing.End(stageSpan, nil)
		recordBidGatewayStage("redis_engine", mode, "ok", time.Since(stageStart))
		totalOutcome = "ok"
	}
	if err == nil {
		h.maybeCreateAutoCommentary(result)
	}
	writeBidAdmissionResult(w, r, result, err)
	return
}

func (h AuctionHandler) maybeCreateAutoCommentary(result auction.BidResponse) {
	if h.AIRepo == nil || result.AuctionID == "" || result.Seq <= 0 {
		return
	}
	eventType := ""
	switch result.Result {
	case auction.BidResultAccepted, auction.BidResultAcceptedExtended:
		eventType = "bid_accepted"
	case auction.BidResultAcceptedSold:
		eventType = "auction_sold"
	default:
		return
	}
	gen := h.AIGen
	if gen == nil {
		gen = aicap.DeterministicGenerator{}
	}
	req := aicap.CommentaryRequest{
		AuctionID:           result.AuctionID,
		SourceSeq:           result.Seq,
		EventType:           eventType,
		CurrentPriceCents:   result.CurrentPriceCents,
		CurrentWinnerMasked: maskPublicUserIDPtr(result.CurrentWinnerID),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, _, err := h.AIRepo.EnqueueAutoCommentary(ctx, req); err != nil {
			fallbackCtx, fallbackCancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer fallbackCancel()
			_, _, _ = h.AIRepo.CreateAutoCommentary(fallbackCtx, gen, req)
		}
	}()
}

func maskPublicUserIDPtr(userID *string) string {
	if userID == nil {
		return ""
	}
	return maskPublicUserID(*userID)
}

func maskPublicUserID(userID string) string {
	if userID == "" {
		return ""
	}
	runes := []rune(userID)
	if len(runes) <= 2 {
		return userID + "**"
	}
	return string(runes[:2]) + "**"
}

func recordBidGatewayStage(stage string, mode string, outcome string, elapsed time.Duration) {
	observability.Observe("auction_bid_gateway_stage_seconds", elapsed.Seconds(), map[string]string{
		"stage":   stage,
		"mode":    mode,
		"outcome": outcome,
	}, observability.DefaultLatencyBuckets)
}

func (h AuctionHandler) DemoCompetingBid(w http.ResponseWriter, r *http.Request) {
	if h.Config.AppEnv != "local" && h.Config.AppEnv != "test" {
		writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "demo bid driver is local/test only", http.StatusForbidden))
		return
	}
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	var req demoCompetingBidRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	state, err := h.Repo.GetAuction(r.Context(), auctionID)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	if req.Mode == "rival_max_bid" {
		if err := h.ensureDemoBidderReady(r.Context(), auctionID, "user_2"); err != nil {
			writeResult(w, r, http.StatusOK, nil, err)
			return
		}
		result, err := h.setDemoRivalMaxBid(r.Context(), auctionID, state, req.AmountCents)
		writeResult(w, r, http.StatusOK, result, err)
		return
	}
	if req.BidderID == "" {
		req.BidderID = demoBidderForMode(req.Mode, state)
	}
	if req.BidderID == "" {
		req.BidderID = "user_2"
	}
	if req.ClientBidID == "" {
		req.ClientBidID = "host-demo-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if req.AmountCents <= 0 {
		req.AmountCents = demoBidAmountForMode(req.Mode, state)
	}
	if err := h.ensureDemoBidderReady(r.Context(), auctionID, req.BidderID); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	if req.Mode == "stale_low" {
		req.AmountCents = state.CurrentPriceCents
		if state.AcceptedBidCount == 0 {
			req.AmountCents = state.StartPriceCents
		}
		if state.Seq > 0 {
			req.ClientSeenSeq = state.Seq - 1
		}
	}
	if req.ClientSeenSeq <= 0 {
		req.ClientSeenSeq = state.Seq
	}
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), AuthUser{ID: req.BidderID, Role: "user"}, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	input := auction.BidInput{
		ClientBidID:   req.ClientBidID,
		AmountCents:   req.AmountCents,
		ClientSeenSeq: req.ClientSeenSeq,
	}
	if h.Engine == nil {
		err := apierrors.New(apierrors.CodeEnginePaused, "redis/kafka hot engine is required", http.StatusServiceUnavailable)
		writeBidAdmissionResult(w, r, auction.BidResponse{}, err)
		return
	}
	result, err := h.Engine.PlaceBid(r.Context(), auctionID, req.BidderID, req.ClientBidID, input, traceID(r.Context()),
		redisx.ACLMembershipKey(auctionID, req.BidderID))
	if err == nil && result.Result == string(apierrors.CodeFatFingerConfirmRequired) && result.ConfirmToken != "" {
		result, err = h.Engine.ConfirmBid(r.Context(), auctionID, req.BidderID, req.ClientBidID, auction.ConfirmBidInput{
			ConfirmToken:   result.ConfirmToken,
			IdempotencyKey: req.ClientBidID,
		}, traceID(r.Context()))
	}
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ensureDemoBidderReady(ctx context.Context, auctionID string, userID string) error {
	if h.Deps == nil || h.Deps.Postgres == nil || userID == "" {
		return nil
	}
	var roomID string
	if err := h.Deps.Postgres.QueryRow(ctx, `SELECT room_id FROM auctions WHERE id = $1`, auctionID).Scan(&roomID); err != nil {
		if err == pgx.ErrNoRows {
			return apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound)
		}
		return err
	}
	displayName := "演示对手"
	if userID == "user_3" {
		displayName = "演示挑战者"
	}
	if _, err := h.Deps.Postgres.Exec(ctx, `
		INSERT INTO users (id, role, display_name)
		VALUES ($1, 'user', $2)
		ON CONFLICT (id) DO NOTHING
	`, userID, displayName); err != nil {
		return err
	}
	_, err := h.Deps.Postgres.Exec(ctx, `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, $2, 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET role = 'viewer',
		              status = 'ACTIVE'
	`, roomID, userID)
	if err != nil {
		return err
	}
	if h.Deps.Redis != nil {
		_ = h.Deps.Redis.Set(ctx, redisx.ACLMembershipKey(auctionID, userID), roomID, 30*time.Minute).Err()
	}
	return nil
}

func (h AuctionHandler) setDemoRivalMaxBid(ctx context.Context, auctionID string, state auction.Auction, requestedMaxAmount int64) (auction.MaxBidIntentResponse, error) {
	userID := "user_2"
	maxAmount := state.CurrentPriceCents + state.IncrementCents*3
	if state.AcceptedBidCount == 0 {
		maxAmount = state.StartPriceCents + state.IncrementCents*3
	}
	if requestedMaxAmount > 0 {
		maxAmount = requestedMaxAmount
	}
	if state.CapPriceCents != nil && maxAmount > *state.CapPriceCents {
		maxAmount = *state.CapPriceCents
	}
	if maxAmount <= state.CurrentPriceCents {
		return auction.MaxBidIntentResponse{}, apierrors.New(apierrors.CodeMaxBidTooLow, "demo max bid has no room above current price", http.StatusConflict)
	}
	intentKey := "host-demo-max-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	result, err := h.Repo.PutMaxBidIntent(ctx, auctionID, userID, intentKey, auction.MaxBidIntentInput{
		MaxAmountCents: maxAmount,
		Source:         auction.MaxBidIntentSourceMaxBid,
	})
	return result, err
}

func demoBidderForMode(mode string, state auction.Auction) string {
	switch mode {
	case "buyer":
		return "user_1"
	case "stale_low":
		return "user_1"
	case "challenge":
		if state.CurrentWinnerID != nil && *state.CurrentWinnerID == "user_3" {
			return "user_2"
		}
		return "user_3"
	case "outbid", "extend", "sold":
		if state.CurrentWinnerID == nil || *state.CurrentWinnerID != "user_2" {
			return "user_2"
		}
		return "user_3"
	default:
		if state.CurrentWinnerID != nil && *state.CurrentWinnerID == "user_2" {
			return "user_3"
		}
		return "user_2"
	}
}

func demoBidAmountForMode(mode string, state auction.Auction) int64 {
	current := state.CurrentPriceCents
	increment := state.IncrementCents
	if increment <= 0 {
		increment = 1
	}
	if mode == "reject" {
		return current + increment + 1
	}
	if mode == "sold" && state.CapPriceCents != nil && *state.CapPriceCents > current {
		return *state.CapPriceCents
	}
	next := current + increment
	if state.AcceptedBidCount == 0 {
		next = state.StartPriceCents + increment
	}
	if mode == "challenge" {
		next = current + increment*2
		if state.AcceptedBidCount == 0 {
			next = state.StartPriceCents + increment*2
		}
	}
	if state.CapPriceCents != nil && next > *state.CapPriceCents {
		return *state.CapPriceCents
	}
	return next
}

func (h AuctionHandler) ResetDemoSeed(w http.ResponseWriter, r *http.Request) {
	if h.Config.AppEnv != "local" && h.Config.AppEnv != "test" {
		writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "demo seed reset is local/test only", http.StatusForbidden))
		return
	}
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	if err := h.ACL.requireHostOwnsRoom(r.Context(), user, "room_main", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	if err := demo.SeedP0Smoke(r.Context(), h.Deps.Postgres, h.Deps.Redis); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "demo seed reset failed: "+err.Error(), http.StatusInternalServerError))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"rooms":       []string{"room_main", "room_side"},
		"active_room": "room_main",
		"active_id":   "auc_live",
	})
}

func (h AuctionHandler) DemoShortenEnd(w http.ResponseWriter, r *http.Request) {
	if h.Config.AppEnv != "local" && h.Config.AppEnv != "test" {
		writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "demo countdown accelerator is local/test only", http.StatusForbidden))
		return
	}
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireHostOwnsAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	req := demoShortenEndRequest{RemainingSeconds: 15}
	if r.Body != nil {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
			return
		}
	}
	result, err := h.Repo.ShortenActiveAuction(r.Context(), auctionID, req.RemainingSeconds, traceID(r.Context()))
	if err == nil {
		err = h.syncDemoShortenedAuctionToRedis(r.Context(), result)
	}
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) syncDemoShortenedAuctionToRedis(ctx context.Context, result auction.Auction) error {
	if h.Deps == nil || h.Deps.Redis == nil || result.EndAt == nil {
		return nil
	}
	endAtMS := result.EndAt.UTC().UnixMilli()
	remainingExtendCount := result.Rule.MaxExtendCount - result.ExtendCount
	if remainingExtendCount < 0 {
		remainingExtendCount = 0
	}
	absoluteEndMS := endAtMS + int64(remainingExtendCount)*int64(result.Rule.ExtendBySeconds)*int64(time.Second/time.Millisecond)
	nowMS := time.Now().UTC().UnixMilli()
	stateKey := redisx.BidEngineStateKey(result.ID)
	guardKey := redisx.BidGuardProjectionKey(result.ID)

	pipe := h.Deps.Redis.TxPipeline()
	pipe.HSet(ctx, stateKey, "end_at_ms", endAtMS, "absolute_end_ms", absoluteEndMS, "paused", "0", "pause_reason", "")
	pipe.PExpire(ctx, stateKey, 24*time.Hour)
	pipe.HSet(ctx, guardKey, "status", string(result.Status), "current_price_cents", result.CurrentPriceCents, "end_at_ms", endAtMS, "seq", result.Seq, "accepted_bid_count", result.AcceptedBidCount, "projected_at_ms", nowMS)
	if result.CurrentWinnerID != nil {
		pipe.HSet(ctx, guardKey, "current_winner_id", *result.CurrentWinnerID)
	}
	pipe.Expire(ctx, guardKey, redisGuardProjectionTTL)
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		for _, cmd := range cmds {
			if cmdErr := cmd.Err(); cmdErr != nil && !errors.Is(cmdErr, redis.Nil) {
				return cmdErr
			}
		}
	}
	return err
}

func (h AuctionHandler) ConfirmBid(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.ConfirmBidInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	if h.Bids != nil {
		if replay, permit, ok, err := h.Bids.admitConfirm(r.Context(), r, user, auctionID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context())); err != nil || ok {
			writeBidAdmissionResult(w, r, replay, err)
			return
		} else if permit != nil {
			defer permit.Release()
		}
	}
	if h.Engine == nil {
		err := apierrors.New(apierrors.CodeEnginePaused, "redis/kafka hot engine is required", http.StatusServiceUnavailable)
		writeBidAdmissionResult(w, r, auction.BidResponse{}, err)
		return
	}
	result, err := h.Engine.ConfirmBid(r.Context(), auctionID, user.ID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context()))
	writeBidAdmissionResult(w, r, result, err)
}

func (h AuctionHandler) executeBidLane(ctx context.Context, auctionID string, userID string, traceID string, fn bidLaneFunc) (auction.BidResponse, error) {
	if h.Lanes == nil {
		return fn(ctx)
	}
	return h.Lanes.Execute(ctx, auctionID, userID, traceID, fn)
}

func (h AuctionHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	result, err := h.Repo.GetLeaderboard(r.Context(), auctionID, user.ID, limit)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) GetMaxBidIntent(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.GetMaxBidIntent(r.Context(), auctionID, user.ID)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) PutMaxBidIntent(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.MaxBidIntentInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.PutMaxBidIntent(r.Context(), auctionID, user.ID, r.Header.Get("Idempotency-Key"), req)
	if err == nil {
		h.invalidateMaxBidIntentSnapshotCache(r.Context(), auctionID, user.ID)
		if trigger, ok := h.triggerMaxBidIntentNow(r.Context(), result.Intent, req, traceID(r.Context())); ok {
			result.TriggerBid = &trigger
		}
	}
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) triggerMaxBidIntentNow(ctx context.Context, intent auction.MaxBidIntent, req auction.MaxBidIntentInput, traceID string) (auction.BidResponse, bool) {
	if h.Engine == nil || h.Repo == nil {
		return auction.BidResponse{}, false
	}
	auctionID := intent.AuctionID
	userID := intent.UserID
	state, err := h.Repo.GetAuction(ctx, auctionID)
	if err != nil || state.Status != auction.StatusActive || (state.CurrentWinnerID != nil && *state.CurrentWinnerID == userID) {
		return auction.BidResponse{}, false
	}
	next := state.CurrentPriceCents + state.IncrementCents
	if state.AcceptedBidCount <= 0 {
		next = state.StartPriceCents + state.IncrementCents
	}
	if req.MaxAmountCents < next {
		return auction.BidResponse{}, false
	}
	if state.CapPriceCents != nil && *state.CapPriceCents > 0 && next > *state.CapPriceCents {
		next = *state.CapPriceCents
	}
	clientBidID := fmt.Sprintf("auto-intent:%s:%s:%d", userID, auctionID, time.Now().UTC().UnixNano())
	resp, err := h.Engine.PlaceActiveMaxBidIntent(ctx, auctionID, userID, intent.ID, clientBidID, auction.BidInput{
		ClientBidID:   clientBidID,
		AmountCents:   next,
		ClientSeenSeq: state.Seq,
	}, traceID, redisx.ACLMembershipKey(auctionID, userID))
	if err != nil || (resp.Result != auction.BidResultEngineAccepted && resp.Result != auction.BidResultEngineSold) {
		return auction.BidResponse{}, false
	}
	return resp, true
}

func (h AuctionHandler) DeleteMaxBidIntent(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	auctionID := chi.URLParam(r, "id")
	if _, err := h.ACL.requireActiveMembershipForAuction(r.Context(), user, auctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.DeleteMaxBidIntent(r.Context(), auctionID, user.ID, r.Header.Get("Idempotency-Key"))
	if err == nil {
		h.invalidateMaxBidIntentSnapshotCache(r.Context(), auctionID, user.ID)
	}
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) PayMock(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.PaymentInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	secret := h.Config.FakePaymentWebhookSecret
	if secret == "" {
		secret = auction.DefaultFakePaymentWebhookSecret
	}
	result, err := h.Repo.PayMockWithSecret(r.Context(), chi.URLParam(r, "id"), user.ID, r.Header.Get("Idempotency-Key"), req, secret, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) FakePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	var req auction.ProviderPaymentWebhook
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	secret := h.Config.FakePaymentWebhookSecret
	if secret == "" {
		secret = auction.DefaultFakePaymentWebhookSecret
	}
	result, err := h.Repo.HandleProviderWebhook(r.Context(), req, secret, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	result, err := h.Repo.ListOrdersFiltered(r.Context(), user.ID, user.Role, r.URL.Query().Get("auction_id"), parsePositiveQueryInt(r, "limit", 100))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ListUserOrders(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	result, err := h.Repo.ListOrdersFiltered(r.Context(), user.ID, user.Role, r.URL.Query().Get("auction_id"), parsePositiveQueryInt(r, "limit", 100))
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": auction.ToOrderHistoryRows(result)})
}

func (h AuctionHandler) ListBidHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	result, err := h.Repo.ListBidHistoryFiltered(r.Context(), user.ID, r.URL.Query().Get("auction_id"), parsePositiveQueryInt(r, "limit", 20))
	writeResult(w, r, http.StatusOK, map[string]any{"items": result}, err)
}

func parsePositiveQueryInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (h AuctionHandler) CreateWSTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	releaseTicket, ok := h.RT.Admission().TryTicket()
	if !ok {
		h.RT.Admission().WriteRejected(w)
		return
	}
	defer releaseTicket()
	var req struct {
		RoomID    string `json:"room_id"`
		AuctionID string `json:"auction_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	if err := h.RT.ValidateRoomAuction(r.Context(), req.RoomID, req.AuctionID); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	if err := h.ACL.requireActiveMembership(r.Context(), user, req.RoomID, req.AuctionID, traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	token, err := h.RT.TicketStore().Issue(r.Context(), realtime.Ticket{
		UserID:    user.ID,
		Role:      user.Role,
		RoomID:    req.RoomID,
		AuctionID: req.AuctionID,
	})
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":        token,
		"expires_in_ms": int(realtime.TicketTTL / time.Millisecond),
	})
}

func (h AuctionHandler) CreateChatMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.CreateChatInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	result, err := h.Repo.CreateChatMessage(r.Context(), roomID, user.ID, req, traceID(r.Context()))
	writeResult(w, r, http.StatusCreated, result, err)
}

func (h AuctionHandler) ListChatMessages(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	roomID := chi.URLParam(r, "room_id")
	if err := h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	result, err := h.Repo.ListChatMessages(r.Context(), roomID, limit)
	writeResult(w, r, http.StatusOK, map[string]any{"items": result}, err)
}

func (h AuctionHandler) requireRoomListAccess(r *http.Request, user AuthUser, roomID string) error {
	if user.Role == "host" {
		return h.ACL.requireHostOwnsRoom(r.Context(), user, roomID, traceID(r.Context()))
	}
	return h.ACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context()))
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func hasAPIErrorCode(err error, code apierrors.Code) bool {
	var apiErr apierrors.APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func auctionHTTPSnapshotCacheKey(auctionID string) string {
	return "auction:http-snapshot:{" + auctionID + "}"
}

func maxBidIntentAbsentCacheKey(auctionID string, userID string) string {
	return "auction:max-bid-intent:absent:{" + auctionID + "}:" + userID
}

func currentUnixMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func writeResult(w http.ResponseWriter, r *http.Request, status int, payload any, err error) {
	if err == nil {
		writeJSON(w, status, payload)
		return
	}
	var apiErr apierrors.APIError
	if errors.As(err, &apiErr) {
		if payload != nil && apiErr.Status == http.StatusAccepted {
			if apiErr.TraceID == "" {
				apiErr.TraceID = traceID(r.Context())
			}
			writeJSON(w, apiErr.Status, payload)
			return
		}
		writeError(w, r, apiErr)
		return
	}
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, minioErr.Message, 500))
		return
	}
	writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "internal server error: "+err.Error(), 500))
}
