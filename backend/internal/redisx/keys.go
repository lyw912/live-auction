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

func WSTicketKey(token string) string {
	return "ws_ticket:" + token
}
