package redisx

import "fmt"

func BidLimitUserKey(auctionID string, userID string) string {
	return fmt.Sprintf("bid:{%s}:limit:user:%s", auctionID, userID)
}

func BidLimitIPKey(auctionID string, ip string) string {
	return fmt.Sprintf("bid:{%s}:limit:ip:%s", auctionID, ip)
}

func BidLimitAuctionKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:limit:auction", auctionID)
}

func BidGuardProjectionKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:guard:projection", auctionID)
}

func BidEngineStateKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:engine:state", auctionID)
}

func BidEngineIdempotencyKey(auctionID string, clientBidID string) string {
	return fmt.Sprintf("bid:{%s}:engine:idem:%s", auctionID, clientBidID)
}

func BidEnginePendingKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:engine:pending", auctionID)
}

func BidEngineAppendMarkerKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:engine:append-marker", auctionID)
}

func BidEngineAppendStatsKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:engine:append-stats", auctionID)
}

func BidEnginePendingAuctionsKey() string {
	return "bid:engine:pending:auctions"
}

// BidEngineLogStreamKey is the append-only Redis Stream that is the in-memory WAL
// for all bid decisions for a single auction. The hash tag {auctionID} pins it to
// the same Redis Cluster slot as the engine state, idempotency, and pending keys so
// Lua scripts can touch all of them atomically.
func BidEngineLogStreamKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:engine:log", auctionID)
}

// BidEngineRelayCursorKey stores the last Redis Stream entry ID that has been
// successfully batch-produced to Kafka. The relay reads forward from this cursor.
func BidEngineRelayCursorKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:engine:relay-cursor", auctionID)
}

func WSTicketKey(token string) string {
	return "ws_ticket:" + token
}

// AuthSessionKey caches an authenticated AuthUser keyed by token hash.
// TTL is capped at 5 minutes so revoked sessions expire quickly.
func AuthSessionKey(tokenHash string) string {
	return "auth:session:" + tokenHash
}

// ACLMembershipKey caches a positive room-membership result for one user/auction
// pair. The {auctionID} hash tag keeps this slot-adjacent to the engine state so
// a future Lua extension could read it atomically if needed.
func ACLMembershipKey(auctionID, userID string) string {
	return fmt.Sprintf("acl:membership:{%s}:%s", auctionID, userID)
}

// BidEnginePendingConfirmKey holds fat-finger pending-confirmation state for one
// (auction, user, clientBidID) triple. Keyed within the auction hash tag so it
// is slot-adjacent to the engine state and can be touched atomically in Lua.
func BidEnginePendingConfirmKey(auctionID, userID, clientBidID string) string {
	return fmt.Sprintf("bid:{%s}:engine:pending_confirm:%s:%s", auctionID, userID, clientBidID)
}
