package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/observability"
	apierrors "live-auction/backend/internal/platform/errors"
	"live-auction/backend/internal/redisx"
)

type roomACL struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func newRoomACL(db *pgxpool.Pool, rdb *redis.Client) roomACL {
	return roomACL{db: db, rdb: rdb}
}

func (a roomACL) requireHostOwnsRoom(ctx context.Context, user AuthUser, roomID string, traceID string) error {
	if user.Role != "host" {
		return a.forbidden(ctx, user, roomID, "", traceID, "host role required")
	}
	var exists bool
	if err := a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rooms WHERE id = $1 AND host_id = $2 AND status = 'OPEN')`, roomID, user.ID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return a.forbidden(ctx, user, roomID, "", traceID, "host does not own room")
	}
	return nil
}

func (a roomACL) requireHostOwnsAuction(ctx context.Context, user AuthUser, auctionID string, traceID string) (string, error) {
	var roomID string
	var ownerID string
	err := a.db.QueryRow(ctx, `
		SELECT a.room_id, r.host_id
		FROM auctions a
		JOIN rooms r ON r.id = a.room_id
		WHERE a.id = $1 AND r.status = 'OPEN'
	`, auctionID).Scan(&roomID, &ownerID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound)
		}
		return "", err
	}
	if user.Role != "host" || ownerID != user.ID {
		return roomID, a.forbidden(ctx, user, roomID, auctionID, traceID, "host does not own room")
	}
	return roomID, nil
}

func (a roomACL) requireActiveMembership(ctx context.Context, user AuthUser, roomID string, auctionID string, traceID string) error {
	if user.Role == "host" {
		return a.requireHostOwnsRoom(ctx, user, roomID, traceID)
	}
	var exists bool
	err := a.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM room_memberships
			WHERE room_id = $1
			  AND user_id = $2
			  AND role IN ('viewer','host')
			  AND status = 'ACTIVE'
		)
	`, roomID, user.ID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return a.forbidden(ctx, user, roomID, auctionID, traceID, "active room membership required")
	}
	return nil
}

func (a roomACL) requireActiveMembershipForAuction(ctx context.Context, user AuthUser, auctionID string, traceID string) (string, error) {
	cacheKey := redisx.ACLMembershipKey(auctionID, user.ID)

	if a.rdb != nil {
		if roomID, err := a.rdb.Get(ctx, cacheKey).Result(); err == nil {
			observability.Inc("auction_acl_membership_cache_total", map[string]string{"result": "hit"})
			return roomID, nil
		} else if err == redis.Nil {
			observability.Inc("auction_acl_membership_cache_total", map[string]string{"result": "miss"})
		} else {
			observability.Inc("auction_acl_membership_cache_total", map[string]string{"result": "error"})
		}
	}

	var roomID string
	var allowed bool
	var err error
	if user.Role == "host" {
		err = a.db.QueryRow(ctx, `
			SELECT a.room_id,
			       EXISTS(
			         SELECT 1
			         FROM rooms r
			         WHERE r.id = a.room_id
			           AND r.host_id = $2
			           AND r.status = 'OPEN'
			       )
			FROM auctions a
			WHERE a.id = $1
		`, auctionID, user.ID).Scan(&roomID, &allowed)
	} else {
		err = a.db.QueryRow(ctx, `
			SELECT a.room_id,
			       EXISTS(
			         SELECT 1
			         FROM room_memberships rm
			         JOIN rooms r ON r.id = rm.room_id
			         WHERE rm.room_id = a.room_id
			           AND rm.user_id = $2
			           AND rm.role IN ('viewer','host')
			           AND rm.status = 'ACTIVE'
			           AND r.status = 'OPEN'
			       )
			FROM auctions a
			WHERE a.id = $1
		`, auctionID, user.ID).Scan(&roomID, &allowed)
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			observability.Inc("auction_acl_membership_cache_total", map[string]string{"result": "db_not_found"})
			return "", apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound)
		}
		observability.Inc("auction_acl_membership_cache_total", map[string]string{"result": "db_error"})
		return "", err
	}
	if !allowed {
		observability.Inc("auction_acl_membership_cache_total", map[string]string{"result": "db_denied"})
		return roomID, a.forbidden(ctx, user, roomID, auctionID, traceID, "active room membership required")
	}
	observability.Inc("auction_acl_membership_cache_total", map[string]string{"result": "db_allowed"})

	// Cache positive results only; denials are rare and we still want fresh checks.
	// TTL = 5min, matching auth session cache cap, so warmup cache survives the
	// burst_wait_ms barrier (40s) without expiring before the burst fires.
	if a.rdb != nil {
		a.rdb.Set(ctx, cacheKey, roomID, 5*time.Minute)
	}
	return roomID, nil
}

func (a roomACL) accessibleRoomIDs(ctx context.Context, user AuthUser) ([]string, error) {
	var rows pgx.Rows
	var err error
	if user.Role == "host" {
		rows, err = a.db.Query(ctx, `SELECT id FROM rooms WHERE host_id = $1 AND status = 'OPEN' ORDER BY created_at DESC`, user.ID)
	} else {
		rows, err = a.db.Query(ctx, `
			SELECT room_id
			FROM room_memberships
			WHERE user_id = $1
			  AND role IN ('viewer','host')
			  AND status = 'ACTIVE'
			ORDER BY joined_at DESC
		`, user.ID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roomIDs []string
	for rows.Next() {
		var roomID string
		if err := rows.Scan(&roomID); err != nil {
			return nil, err
		}
		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, rows.Err()
}

func (a roomACL) roomForAuction(ctx context.Context, auctionID string) (string, error) {
	var roomID string
	if err := a.db.QueryRow(ctx, `SELECT room_id FROM auctions WHERE id = $1`, auctionID).Scan(&roomID); err != nil {
		if err == pgx.ErrNoRows {
			return "", apierrors.New(apierrors.CodeAuctionNotFound, "auction not found", http.StatusNotFound)
		}
		return "", err
	}
	return roomID, nil
}

func (a roomACL) forbidden(ctx context.Context, user AuthUser, roomID string, auctionID string, traceID string, reason string) error {
	_ = a.recordACLReject(ctx, user, roomID, auctionID, traceID, reason)
	return apierrors.New(apierrors.CodeForbiddenRoom, reason, http.StatusForbidden)
}

func (a roomACL) recordACLReject(ctx context.Context, user AuthUser, roomID string, auctionID string, traceID string, reason string) error {
	if a.db == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"room_id":    roomID,
		"auction_id": auctionID,
		"user_id":    user.ID,
		"role":       user.Role,
		"trace_id":   traceID,
		"reason":     reason,
	})
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
		VALUES ('MED', 'ACL_FORBIDDEN', NULLIF($1, ''), $2, $3)
	`, auctionID, "room ACL rejected request", payload)
	return err
}
