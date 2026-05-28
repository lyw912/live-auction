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

func BidEngineStreamKey(auctionID string) string {
	return fmt.Sprintf("bid:{%s}:engine:stream", auctionID)
}

func WSTicketKey(token string) string {
	return "ws_ticket:" + token
}
