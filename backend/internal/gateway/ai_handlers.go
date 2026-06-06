package gateway

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	aicap "live-auction/backend/internal/ai"
	apierrors "live-auction/backend/internal/platform/errors"
)

type AIHandler struct {
	Repo    *aicap.Repository
	Gen     aicap.Generator
	RoomACL roomACL
}

func (h AIHandler) CreateListingDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req aicap.ListingDraftRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	job, err := h.Repo.CreateListingDraft(r.Context(), user.ID, h.generator(), req)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

func (h AIHandler) GetListingDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	job, err := h.Repo.GetJob(r.Context(), user.ID, chi.URLParam(r, "job_id"))
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h AIHandler) ApplyListingDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	job, err := h.Repo.MarkJobApplied(r.Context(), user.ID, chi.URLParam(r, "job_id"))
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h AIHandler) CreateCommentary(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	var req aicap.CommentaryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	msg, job, err := h.Repo.CreateCommentary(r.Context(), user.ID, h.generator(), req)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"message": msg, "job": job})
}

func (h AIHandler) ListSystemMessages(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "room_id")
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	if roomID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "room_id is required", http.StatusBadRequest))
		return
	}
	if err := h.RoomACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.Repo.ListSystemMessages(r.Context(), roomID, limit)
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "system messages query failed", http.StatusInternalServerError))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h AIHandler) EvaluateSentinel(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	alerts, err := h.Repo.EvaluateSentinel(r.Context(), user.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": alerts})
}

func (h AIHandler) ListSentinelAlerts(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	alerts, err := h.Repo.ListRiskAlerts(r.Context(), user.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": alerts})
}

func (h AIHandler) BuildRecap(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	recap, job, err := h.Repo.BuildAuctionRecap(r.Context(), user.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recap": recap, "job": job})
}

func (h AIHandler) ProductQA(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "room_id")
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	if roomID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "room_id is required", http.StatusBadRequest))
		return
	}
	if err := h.RoomACL.requireActiveMembership(r.Context(), user, roomID, "", traceID(r.Context())); err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	var req aicap.ProductQARequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	answer, err := h.Repo.AnswerProductQuestion(r.Context(), roomID, req)
	if err != nil {
		writeResult(w, r, http.StatusOK, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

func (h AIHandler) generator() aicap.Generator {
	if h.Gen == nil {
		return aicap.DeterministicGenerator{}
	}
	return h.Gen
}
