package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	changedSource := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":25000,"client_seen_seq":0,"source":"PRE_BID"}`, "max-intent-1", "user_1")
	if changedSource.Code != http.StatusConflict {
		t.Fatalf("changed-source key status = %d, want 409 body=%s", changedSource.Code, changedSource.Body.String())
	}
	assertAPIErrorCode(t, changedSource, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest)

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

func TestMaxBidIntentRoutesRejectUnsafeAmountsAndTerminalAuctions(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_max_bid_abuse_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	if _, err := db.Exec(context.Background(), `UPDATE auctions SET cap_price_cents = 40000 WHERE id = $1`, row.ID); err != nil {
		t.Fatalf("set cap price: %v", err)
	}

	cases := []struct {
		name string
		body string
		want apierrors.Code
	}{
		{name: "below-current-minimum", body: `{"max_amount_cents":1,"client_seen_seq":0}`, want: apierrors.CodeMaxBidTooLow},
		{name: "off-grid", body: `{"max_amount_cents":25001,"client_seen_seq":0}`, want: apierrors.CodeMaxBidIncrementMismatch},
		{name: "above-cap", body: `{"max_amount_cents":45000,"client_seen_seq":0}`, want: apierrors.CodeMaxBidAboveCap},
		{name: "invalid-source", body: `{"max_amount_cents":25000,"client_seen_seq":0,"source":"BOT"}`, want: apierrors.CodeInvalidArgument},
	}
	for _, tc := range cases {
		rec := performMaxBidIntent(router, http.MethodPut, row.ID, tc.body, "max-intent-abuse-"+tc.name, "user_1")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400 body=%s", tc.name, rec.Code, rec.Body.String())
		}
		assertAPIErrorCode(t, rec, tc.want)
	}

	accepted := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":25000,"client_seen_seq":0,"source":"MAX_BID"}`, "max-intent-before-terminal", "user_1")
	if accepted.Code != http.StatusOK {
		t.Fatalf("accepted PUT status = %d body=%s", accepted.Code, accepted.Body.String())
	}
	if _, err := db.Exec(context.Background(), `UPDATE auctions SET status = 'CANCELLED' WHERE id = $1`, row.ID); err != nil {
		t.Fatalf("force terminal auction: %v", err)
	}
	terminalPut := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":30000,"client_seen_seq":0,"source":"MAX_BID"}`, "max-intent-terminal-put", "user_1")
	if terminalPut.Code != http.StatusConflict {
		t.Fatalf("terminal PUT status = %d, want 409 body=%s", terminalPut.Code, terminalPut.Body.String())
	}
	assertAPIErrorCode(t, terminalPut, apierrors.CodeAuctionNotActive)
	terminalDelete := performMaxBidIntent(router, http.MethodDelete, row.ID, "", "max-intent-terminal-delete", "user_1")
	if terminalDelete.Code != http.StatusOK {
		t.Fatalf("terminal DELETE status = %d, want 200 body=%s", terminalDelete.Code, terminalDelete.Body.String())
	}
	var terminalDeleteResp auction.MaxBidIntentResponse
	if err := json.Unmarshal(terminalDelete.Body.Bytes(), &terminalDeleteResp); err != nil {
		t.Fatalf("decode terminal DELETE response: %v body=%s", err, terminalDelete.Body.String())
	}
	if terminalDeleteResp.Intent.Status != auction.MaxBidIntentStatusCancelled {
		t.Fatalf("terminal DELETE intent status = %s, want CANCELLED", terminalDeleteResp.Intent.Status)
	}
}

func TestMaxBidIntentRoutesBoundProcessingAndChurn(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_max_bid_churn_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	ctx := context.Background()

	stuckHash := maxBidIntentTestHash(row.ID, "user_1", "max-intent-stuck", 25_000, "MAX_BID")
	if _, err := db.Exec(ctx, `
		INSERT INTO idempotency_records (scope_type, scope_id, user_id, idempotency_key, request_hash, status, locked_until)
		VALUES ('max_bid_intent', $1, 'user_1', 'max-intent-stuck', $2, 'PROCESSING', now() + interval '5 minutes')
	`, row.ID, stuckHash); err != nil {
		t.Fatalf("insert processing idempotency: %v", err)
	}
	processing := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":25000,"client_seen_seq":0,"source":"MAX_BID"}`, "max-intent-stuck", "user_1")
	if processing.Code != http.StatusConflict {
		t.Fatalf("processing status = %d, want 409 body=%s", processing.Code, processing.Body.String())
	}
	assertAPIErrorCode(t, processing, apierrors.CodeProcessingRetryLater)
	processingChanged := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":30000,"client_seen_seq":0,"source":"MAX_BID"}`, "max-intent-stuck", "user_1")
	if processingChanged.Code != http.StatusConflict {
		t.Fatalf("processing changed status = %d, want 409 body=%s", processingChanged.Code, processingChanged.Body.String())
	}
	assertAPIErrorCode(t, processingChanged, apierrors.CodeIdempotencyKeyReusedWithDifferentRequest)

	expiredHash := maxBidIntentTestHash(row.ID, "user_1", "max-intent-expired", 25_000, "MAX_BID")
	if _, err := db.Exec(ctx, `
		INSERT INTO idempotency_records (scope_type, scope_id, user_id, idempotency_key, request_hash, status, locked_until)
		VALUES ('max_bid_intent', $1, 'user_1', 'max-intent-expired', $2, 'PROCESSING', now() - interval '1 minute')
	`, row.ID, expiredHash); err != nil {
		t.Fatalf("insert expired idempotency: %v", err)
	}
	expired := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":25000,"client_seen_seq":0,"source":"MAX_BID"}`, "max-intent-expired", "user_1")
	if expired.Code != http.StatusConflict {
		t.Fatalf("expired processing status = %d, want 409 body=%s", expired.Code, expired.Body.String())
	}
	assertAPIErrorCode(t, expired, apierrors.CodeIdempotencyTimeout)

	create := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":25000,"client_seen_seq":0,"source":"MAX_BID"}`, "max-intent-churn-create", "user_1")
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	cancel := performMaxBidIntent(router, http.MethodDelete, row.ID, "", "max-intent-churn-cancel", "user_1")
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d body=%s", cancel.Code, cancel.Body.String())
	}
	recreate := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":30000,"client_seen_seq":0,"source":"PRE_BID"}`, "max-intent-churn-recreate", "user_1")
	if recreate.Code != http.StatusOK {
		t.Fatalf("recreate status = %d body=%s", recreate.Code, recreate.Body.String())
	}
	var intent auction.MaxBidIntentResponse
	if err := json.Unmarshal(recreate.Body.Bytes(), &intent); err != nil {
		t.Fatalf("decode recreate: %v", err)
	}
	if intent.Intent.Status != auction.MaxBidIntentStatusActive || intent.Intent.Source != auction.MaxBidIntentSourcePreBid || intent.Intent.Version < 2 {
		t.Fatalf("recreated intent did not reset active PRE_BID state: %#v", intent.Intent)
	}
}

func TestAuctionSnapshotCarriesOnlyCurrentUserMaxBidIntent(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_max_bid_snapshot_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")
	if _, err := db.Exec(context.Background(), `
		INSERT INTO users (id, role, display_name) VALUES ('user_other_snapshot', 'user', 'Other Snapshot User')
		ON CONFLICT DO NOTHING
	`); err != nil {
		t.Fatalf("insert other user: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES ($1, 'user_other_snapshot', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL
	`, row.RoomID); err != nil {
		t.Fatalf("insert other membership: %v", err)
	}

	if _, err := repo.UpsertMaxBidIntent(context.Background(), row.ID, "user_1", auction.MaxBidIntentInput{MaxAmountCents: 25_000}); err != nil {
		t.Fatalf("UpsertMaxBidIntent: %v", err)
	}

	owner := performAuctionGet(router, row.ID, "user_1")
	if owner.Code != http.StatusOK {
		t.Fatalf("owner snapshot status = %d body=%s", owner.Code, owner.Body.String())
	}
	var ownerPayload map[string]any
	if err := json.Unmarshal(owner.Body.Bytes(), &ownerPayload); err != nil {
		t.Fatalf("decode owner snapshot: %v", err)
	}
	intent, ok := ownerPayload["max_bid_intent"].(map[string]any)
	if !ok {
		t.Fatalf("owner snapshot missing max_bid_intent: %#v", ownerPayload)
	}
	if intent["user_id"] != "user_1" || intent["max_amount_cents"].(float64) != 25_000 {
		t.Fatalf("unexpected owner intent: %#v", intent)
	}

	other := performAuctionGet(router, row.ID, "user_other_snapshot")
	if other.Code != http.StatusOK {
		t.Fatalf("other snapshot status = %d body=%s", other.Code, other.Body.String())
	}
	if bytes.Contains(other.Body.Bytes(), []byte("max_bid_intent")) || bytes.Contains(other.Body.Bytes(), []byte("max_amount_cents")) {
		t.Fatalf("other user snapshot leaked private intent: %s", other.Body.String())
	}

	list := performAuctionList(router, row.RoomID, "user_1")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", list.Code, list.Body.String())
	}
	if bytes.Contains(list.Body.Bytes(), []byte("max_bid_intent")) || bytes.Contains(list.Body.Bytes(), []byte("max_amount_cents")) {
		t.Fatalf("public auction list leaked private intent: %s", list.Body.String())
	}
}

func TestAuctionSnapshotHTTPReadCacheDoesNotLeakMaxBidIntentAndInvalidatesAbsentIntent(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	router := NewRouter(testConfig(), &storage.Dependencies{Postgres: db, Redis: rdb}, slog.Default())
	repo := auction.NewRepository(db)
	row := createACLAuction(t, repo, db, "room_max_bid_snapshot_cache_"+uuid.NewString(), "host_1", "user_1", "ACTIVE")

	first := performAuctionGet(router, row.ID, "user_1")
	if first.Code != http.StatusOK {
		t.Fatalf("first snapshot status = %d body=%s", first.Code, first.Body.String())
	}
	if bytes.Contains(first.Body.Bytes(), []byte("max_bid_intent")) {
		t.Fatalf("empty user snapshot unexpectedly had max_bid_intent: %s", first.Body.String())
	}
	if exists, err := rdb.Exists(context.Background(), auctionHTTPSnapshotCacheKey(row.ID)).Result(); err != nil || exists != 1 {
		t.Fatalf("http snapshot cache exists=%d err=%v", exists, err)
	}
	if cached, err := rdb.Get(context.Background(), maxBidIntentAbsentCacheKey(row.ID, "user_1")).Result(); err != nil || cached != maxBidIntentAbsentCacheSentinel {
		t.Fatalf("absent max intent cache = %q err=%v", cached, err)
	}

	put := performMaxBidIntent(router, http.MethodPut, row.ID, `{"max_amount_cents":25000,"client_seen_seq":0,"source":"MAX_BID"}`, "max-intent-cache-put", "user_1")
	if put.Code != http.StatusOK {
		t.Fatalf("put max intent status = %d body=%s", put.Code, put.Body.String())
	}
	if exists, err := rdb.Exists(context.Background(), maxBidIntentAbsentCacheKey(row.ID, "user_1")).Result(); err != nil || exists != 0 {
		t.Fatalf("absent max intent cache after put exists=%d err=%v", exists, err)
	}

	second := performAuctionGet(router, row.ID, "user_1")
	if second.Code != http.StatusOK {
		t.Fatalf("second snapshot status = %d body=%s", second.Code, second.Body.String())
	}
	if !bytes.Contains(second.Body.Bytes(), []byte(`"max_bid_intent"`)) || !bytes.Contains(second.Body.Bytes(), []byte(`"max_amount_cents":25000`)) {
		t.Fatalf("snapshot did not include current user's intent after invalidation: %s", second.Body.String())
	}
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

func performAuctionGet(router http.Handler, auctionID string, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/auctions/"+auctionID, nil)
	req.Header.Set("X-Mock-Role", "user")
	req.Header.Set("X-Mock-User-Id", userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func performAuctionList(router http.Handler, roomID string, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/auctions?room_id="+roomID, nil)
	req.Header.Set("X-Mock-Role", "user")
	req.Header.Set("X-Mock-User-Id", userID)
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

func maxBidIntentTestHash(auctionID string, userID string, idempotencyKey string, maxAmountCents int64, source string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("max-bid-intent:v2|%s|%s|%s|%d|%s", auctionID, userID, idempotencyKey, maxAmountCents, source)))
	return hex.EncodeToString(sum[:])
}
