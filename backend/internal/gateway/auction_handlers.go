package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
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
}

type uploadURLRequest struct {
	ObjectName  string `json:"object_name"`
	ContentType string `json:"content_type"`
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
	var req auction.CreateAuctionInput
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	result, err := h.Repo.CreateAuction(r.Context(), req, traceID(r.Context()))
	writeResult(w, r, http.StatusCreated, result, err)
}

func (h AuctionHandler) UpdateRules(w http.ResponseWriter, r *http.Request) {
	var req auction.Rule
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", 400))
		return
	}
	result, err := h.Repo.UpdateRules(r.Context(), chi.URLParam(r, "id"), req, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Schedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StartAt *time.Time `json:"start_at"`
	}
	if r.Body != http.NoBody {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	result, err := h.Repo.Schedule(r.Context(), chi.URLParam(r, "id"), req.StartAt, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Unschedule(w http.ResponseWriter, r *http.Request) {
	result, err := h.Repo.Unschedule(r.Context(), chi.URLParam(r, "id"), traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Start(w http.ResponseWriter, r *http.Request) {
	result, err := h.Repo.Start(r.Context(), chi.URLParam(r, "id"), traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	result, err := h.Repo.Cancel(r.Context(), chi.URLParam(r, "id"), traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ListAuctions(w http.ResponseWriter, r *http.Request) {
	result, err := h.Repo.ListAuctions(r.Context(), r.URL.Query().Get("room_id"))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) ListRoomAuctions(w http.ResponseWriter, r *http.Request) {
	result, err := h.Repo.ListAuctions(r.Context(), chi.URLParam(r, "room_id"))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) GetAuction(w http.ResponseWriter, r *http.Request) {
	result, err := h.Repo.GetAuction(r.Context(), chi.URLParam(r, "id"))
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
	result, err := h.Repo.PlaceBid(r.Context(), chi.URLParam(r, "id"), user.ID, r.Header.Get("Idempotency-Key"), req, traceID(r.Context()))
	writeResult(w, r, http.StatusOK, result, err)
}

func (h AuctionHandler) PayMock(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	result, err := h.Repo.PayMock(r.Context(), chi.URLParam(r, "id"), user.ID, r.Header.Get("Idempotency-Key"), traceID(r.Context()))
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

func (h AuctionHandler) CreateWSTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
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
