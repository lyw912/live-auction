package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"live-auction/backend/internal/auction"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/storage"
)

func TestMaxBidIntentRoutesAreCurrentUserScopedAndIdempotent(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_max_bid_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	if _, err := db.Exec(context.Background(), `
		INSERT INTO users (id, role, display_name) VALUES ('user_other_max_bid', 'user', 'Other Max Bid User')
		ON CONFLICT DO NOTHING
	`); err != nil {
		t.Fatalf("insert other user: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'user_other_max_bid', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
	`, row.RoomID); err != nil {
		t.Fatalf("insert other membership: %v", err)
	}

	createBody := `{"max_amount_cents":25000,"client_seen_seq":` + strconv.FormatInt(row.Seq, 10) + `,"source":"MAX_BID"}`
	first := performMaxBidIntent(router, http.MethodPut, row.ID, createBody, "max-intent-1", "user_1")
	if first.Code != http.StatusOK {
		t.Fatalf("first PUT status = %d body=%s", first.Code, first.Body.String())
	}
	var firstResp auction.MaxBidIntentResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if firstResp.Result != auction.MaxBidIntentResultActive || firstResp.Intent.UserID != "user_1" || firstResp.Intent.MaxAmountCents != 25_000 {
		t.Fatalf("unexpected first response: %#v", firstResp)
	}

	replay := performMaxBidIntent(router, http.MethodPut, row.ID, createBody, "max-intent-1", "user_1")
	if replay.Code != http.StatusOK {
		t.Fatalf("replay PUT status = %d body=%s", replay.Code, replay.Body.String())
	}
	var replayResp auction.MaxBidIntentResponse
	if err := json.Unmarshal(replay.Body.Bytes(), &replayResp); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if replayResp.Intent.ID != firstResp.Intent.ID || replayResp.Intent.Version != firstResp.Intent.Version {
		t.Fatalf("idempotent replay mutated intent: got %#v want %#v", replayResp.Intent, firstResp.Intent)
	}

	changed := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":30000,"client_seen_seq":0}`, "max-intent-1", "user_1")
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed key status = %d, want 409 body=%s", changed.Code, changed.Body.String())
	}
	assertAPIErrorCode(t, changed, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest)

	currentUserGet := performMaxBidIntent(router, http.MethodGet, row.ID, "", "", "user_1")
	if currentUserGet.Code != http.StatusOK {
		t.Fatalf("current user GET status = %d body=%s", currentUserGet.Code, currentUserGet.Body.String())
	}
	var got auction.MaxBidIntent
	if err := json.Unmarshal(currentUserGet.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if got.ID != firstResp.Intent.ID || got.UserID != "user_1" {
		t.Fatalf("unexpected GET intent: %#v", got)
	}

	otherUserGet := performMaxBidIntent(router, http.MethodGet, row.ID, "", "", "user_other_max_bid")
	if otherUserGet.Code != http.StatusNotFound {
		t.Fatalf("other user GET status = %d, want 404 body=%s", otherUserGet.Code, otherUserGet.Body.String())
	}

	cancel := performMaxBidIntent(router, http.MethodDelete, row.ID, "", "max-intent-cancel-1", "user_1")
	if cancel.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d body=%s", cancel.Code, cancel.Body.String())
	}
	var cancelResp auction.MaxBidIntentResponse
	if err := json.Unmarshal(cancel.Body.Bytes(), &cancelResp); err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	if cancelResp.Result != auction.MaxBidIntentResultCancelled || cancelResp.Intent.Status != auction.MaxBidIntentStatusCancelled {
		t.Fatalf("unexpected cancel response: %#v", cancelResp)
	}
	cancelReplay := performMaxBidIntent(router, http.MethodDelete, row.ID, "", "max-intent-cancel-1", "user_1")
	if cancelReplay.Code != http.StatusOK {
		t.Fatalf("DELETE replay status = %d body=%s", cancelReplay.Code, cancelReplay.Body.String())
	}
}

func TestMaxBidIntentRoutesRequireMembershipAndIdempotencyKey(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	foreign := createACLAuction(t, repo, db, "room_max_bid_foreign_"+uuid.NewString(), "host_1", "user_1", "BANNED")

	body := `{"max_amount_cents":25000,"client_seen_seq":0}`
	forbidden := performMaxBidIntent(router, http.MethodPut, foreign.ID, body, "max-intent-foreign", "user_1")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("foreign PUT status = %d, want 403 body=%s", forbidden.Code, forbidden.Body.String())
	}

	owned := createACLAuction(t, repo, db, "room_max_bid_owned_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	missingKey := performMaxBidIntent(router, http.MethodPut, owned.ID, body, "", "user_1")
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("missing-key PUT status = %d, want 400 body=%s", missingKey.Code, missingKey.Body.String())
	}
	assertAPIErrorCode(t, missingKey, apierrors.CodeInvalidArgument)
}

func performMaxBidIntent(router http.Handler, method string, auctionID string, body string, key string, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/auctions/"+auctionID+"/max-bid-intent", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mock-Role", "user")
	req.Header.Set("X-Mock-User-Id", userID)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertAPIErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want apierrors.Code) {
	t.Helper()
	var payload apierrors.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode API error: %v body=%s", err, rec.Body.String())
	}
	if payload.Code != want {
		t.Fatalf("error code = %s, want %s body=%s", payload.Code, want, rec.Body.String())
	}
}
