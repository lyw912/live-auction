package redisx

import "testing"

func TestBidEngineLuaKeyTopologyDocumentsClusterBoundary(t *testing.T) {
	auctionID := "auc_cluster_boundary"
	clientBidID := "bid_1"
	auctionSlotKeys := []string{
		BidEngineStateKey(auctionID),
		BidEngineIdempotencyKey(auctionID, clientBidID),
		BidEnginePendingKey(auctionID),
		BidEngineLogStreamKey(auctionID),
		ACLMembershipKey(auctionID, "user_1"),
	}
	slot := redisClusterSlot(auctionSlotKeys[0])
	for _, key := range auctionSlotKeys[1:] {
		if got := redisClusterSlot(key); got != slot {
			t.Fatalf("key %q slot=%d, want auction slot %d", key, got, slot)
		}
	}

	globalKeySlot := redisClusterSlot(BidEnginePendingAuctionsKey())
	if globalKeySlot == slot {
		t.Fatalf("global pending-auctions key unexpectedly hashes to auction slot %d", slot)
	}
}

func redisClusterSlot(key string) int {
	tag := redisHashTag(key)
	return int(crc16([]byte(tag)) % 16384)
}

func redisHashTag(key string) string {
	start := -1
	for i := 0; i < len(key); i++ {
		if key[i] == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return key
	}
	for i := start + 1; i < len(key); i++ {
		if key[i] == '}' {
			if i == start+1 {
				return key
			}
			return key[start+1 : i]
		}
	}
	return key
}

func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
