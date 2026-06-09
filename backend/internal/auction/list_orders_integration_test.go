package auction

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// createTestRoomForHost creates a test room owned by the given hostID.
// The host user must already exist in the DB.
func createTestRoomForHost(t *testing.T, db *pgxpool.Pool, hostID string) string {
	t.Helper()
	roomID := "room_test_" + uuid.NewString()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO rooms (id, host_id, status)
		VALUES ($1, $2, 'OPEN')
	`, roomID, hostID); err != nil {
		t.Fatalf("insert test room for host %s: %v", hostID, err)
	}
	return roomID
}

// ensureHost inserts a host user if not already present.
func ensureHost(t *testing.T, db *pgxpool.Pool, hostID string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO users (id, role, display_name)
		VALUES ($1, 'host', $1)
		ON CONFLICT (id) DO NOTHING
	`, hostID); err != nil {
		t.Fatalf("ensure host user %s: %v", hostID, err)
	}
}

// createActiveAuctionInRoom creates a fully started auction in a specific room.
func createActiveAuctionInRoom(t *testing.T, repo *Repository, db *pgxpool.Pool, roomID string, capPrice *int64) Auction {
	t.Helper()
	item, err := repo.CreateItem(context.Background(), CreateItemInput{Title: "M3 Test Item"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	a, err := repo.CreateAuction(context.Background(), CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  5_000,
		CapPriceCents:   capPrice,
		Rule:            validRule(),
	}, "tr_m3")
	if err != nil {
		t.Fatalf("CreateAuction: %v", err)
	}
	a, err = repo.Schedule(context.Background(), a.ID, nil, "tr_m3")
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	a, err = repo.Start(context.Background(), a.ID, "tr_m3")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return a
}

// driveToSOLD bids at cap price to create an order, returning the auction ID.
func driveToSOLD(t *testing.T, repo *Repository, winnerID string, cap int64) string {
	t.Helper()
	db := repo.db
	a := createActiveAuction(t, repo, db, &cap)
	input := BidInput{ClientBidID: "bid-sold-" + uuid.NewString(), AmountCents: cap}
	if _, err := repo.PlaceBidPostgresLegacyForTests(context.Background(), a.ID, winnerID, input.ClientBidID, input, "tr_m3"); err != nil {
		t.Fatalf("PlaceBid cap: %v", err)
	}
	return a.ID
}

// driveToSOLDInRoom bids at cap in a specified room.
func driveToSOLDInRoom(t *testing.T, repo *Repository, db *pgxpool.Pool, roomID, winnerID string, cap int64) string {
	t.Helper()
	a := createActiveAuctionInRoom(t, repo, db, roomID, &cap)
	input := BidInput{ClientBidID: "bid-sold-" + uuid.NewString(), AmountCents: cap}
	if _, err := repo.PlaceBidPostgresLegacyForTests(context.Background(), a.ID, winnerID, input.ClientBidID, input, "tr_m3"); err != nil {
		t.Fatalf("PlaceBid cap in room: %v", err)
	}
	return a.ID
}

func TestListOrdersHostScopedToOwnRoom(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)

	// Ensure a second host exists.
	ensureHost(t, db, "host_2_m3")

	// Create one room per host and drive each to SOLD.
	room1 := createTestRoomForHost(t, db, "host_1")
	room2 := createTestRoomForHost(t, db, "host_2_m3")

	cap := int64(20_000)
	auction1ID := driveToSOLDInRoom(t, repo, db, room1, "user_1", cap)
	auction2ID := driveToSOLDInRoom(t, repo, db, room2, "user_1", cap)

	// host_1 should see only auction1's order.
	host1Orders, err := repo.ListOrders(ctx, "host_1", "host")
	if err != nil {
		t.Fatalf("ListOrders host_1: %v", err)
	}
	for _, o := range host1Orders {
		if o.AuctionID == auction2ID {
			t.Errorf("host_1 must not see orders from host_2's auction; got auction_id=%s", o.AuctionID)
		}
	}
	found := false
	for _, o := range host1Orders {
		if o.AuctionID == auction1ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("host_1 must see their own auction's order (auction_id=%s)", auction1ID)
	}

	// host_2 should see only auction2's order.
	host2Orders, err := repo.ListOrders(ctx, "host_2_m3", "host")
	if err != nil {
		t.Fatalf("ListOrders host_2: %v", err)
	}
	for _, o := range host2Orders {
		if o.AuctionID == auction1ID {
			t.Errorf("host_2 must not see orders from host_1's auction; got auction_id=%s", o.AuctionID)
		}
	}
	found = false
	for _, o := range host2Orders {
		if o.AuctionID == auction2ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("host_2 must see their own auction's order (auction_id=%s)", auction2ID)
	}
}

func TestListOrdersUserSeesOnlyOwnOrders(t *testing.T) {
	db := openIntegrationDB(t)
	ctx := context.Background()
	repo := NewRepository(db)

	// Two users each win one auction; each must only see their own order.
	userA := createTestUser(t, db)
	userB := createTestUser(t, db)

	cap := int64(20_000)
	aID := driveToSOLD(t, repo, userA, cap)
	bID := driveToSOLD(t, repo, userB, cap)

	aOrders, err := repo.ListOrders(ctx, userA, "user")
	if err != nil {
		t.Fatalf("ListOrders userA: %v", err)
	}
	for _, o := range aOrders {
		if o.WinnerID != userA {
			t.Errorf("userA must only see their own orders; got winner_id=%s", o.WinnerID)
		}
		if o.AuctionID == bID {
			t.Errorf("userA must not see userB's auction order")
		}
	}
	found := false
	for _, o := range aOrders {
		if o.AuctionID == aID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("userA must see their own winning order for auction %s", aID)
	}

	bOrders, err := repo.ListOrders(ctx, userB, "user")
	if err != nil {
		t.Fatalf("ListOrders userB: %v", err)
	}
	for _, o := range bOrders {
		if o.WinnerID != userB {
			t.Errorf("userB must only see their own orders; got winner_id=%s", o.WinnerID)
		}
	}
}
