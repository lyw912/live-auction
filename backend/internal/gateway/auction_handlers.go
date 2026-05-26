package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/realtime"
	"live-auction/backend/internal/storage"
)

type AuctionHandler struct {
	Config config.Config
	Deps   *storage.Dependencies
	Repo   *auction.Repository
	RT     *realtime.Server
	ACL    roomACL
	Bids   *bidAdmission
}

type uploadURLRequest struct {
	ObjectName  string `json:"object_name"`
	ContentType string `json:"content_type"`
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
		"public_url":   "http://" + h.Config.MinIOEndpoint + "/" + h.Deps.Bucket + "/" + req.ObjectName,
		"expires_in_s": 900,
	})
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
	result, err := h.Repo.GetAuction(r.Context(), auctionID)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) PlaceBid(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req auction.BidInput
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
		if replay, permit, ok, err := h.Bids.admit(r.Context(), r, user, auctionID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context())); err != nil || ok {
			writeBidAdmissionResult(w, r, replay, err)
			return
		} else if permit != nil {
			defer permit.Release()
		}
	}
	result, err := h.Repo.PlaceBid(r.Context(), auctionID, user.ID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
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
	result, err := h.Repo.ConfirmBid(r.Context(), auctionID, user.ID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
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
	writeResult(w, r, http.StatusOK, result, err)
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
	result, err := h.Repo.ListOrders(r.Context(), user.ID, user.Role)
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ListUserOrders(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	result, err := h.Repo.ListOrders(r.Context(), user.ID, user.Role)
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
	result, err := h.Repo.ListBidHistory(r.Context(), user.ID)
	writeResult(w, r, http.StatusOK, map[string]any{"items": result}, err)
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

func writeResult(w http.ResponseWriter, r *http.Request, status int, payload any, err error) {
	if err == nil {
		writeJSON(w, status, payload)
		return
	}
	var apiErr apierrors.APIError
	if errors.As(err, &apiErr) {
		writeError(w, r, apiErr)
		return
	}
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, minioErr.Message, 500))
		return
	}
	writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "internal server error", 500))
}
