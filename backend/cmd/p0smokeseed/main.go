package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/config"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := config.Load()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() { _ = rdb.Close() }()

	if err := seed(ctx, db, rdb); err != nil {
		log.Fatalf("seed p0 smoke data: %v", err)
	}
	_, _ = os.Stdout.WriteString("p0 smoke data ready\n")
}

func seed(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		DELETE FROM scheduler_jobs
		WHERE target_id = 'auc_live'
		   OR target_id IN (SELECT id FROM orders WHERE auction_id = 'auc_live');
		DELETE FROM idempotency_records
		WHERE scope_id = 'auc_live'
		   OR scope_id IN (SELECT id FROM orders WHERE auction_id = 'auc_live');
		DELETE FROM bids WHERE auction_id = 'auc_live';
		DELETE FROM outbox_delivery
		WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id = 'auc_live');
		DELETE FROM outbox_events WHERE auction_id = 'auc_live';
		DELETE FROM auction_events WHERE auction_id = 'auc_live';
		DELETE FROM orders WHERE auction_id = 'auc_live';
		DELETE FROM auction_rules WHERE auction_id = 'auc_live';
		DELETE FROM auctions WHERE id = 'auc_live';

		INSERT INTO users (id, role, display_name, city)
		VALUES
		  ('host_1', 'host', 'Demo Host', 'Hangzhou'),
		  ('user_1', 'user', 'Demo User', 'Shanghai'),
		  ('user_2', 'user', 'Prior Leader', 'Beijing'),
		  ('user_3', 'user', 'Smoke Bidder', 'Shenzhen')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO rooms (id, host_id, status)
		VALUES ('room_main', 'host_1', 'OPEN')
		ON CONFLICT (id) DO UPDATE SET host_id = EXCLUDED.host_id, status = EXCLUDED.status;

		INSERT INTO items (id, title, image_url, description, status)
		VALUES (
		  'item_live',
		  'P0 Live Smoke Item',
		  NULL,
		  'P0 live backend smoke item.',
		  'READY'
		)
		ON CONFLICT (id) DO UPDATE
		SET title = EXCLUDED.title,
		    image_url = EXCLUDED.image_url,
		    description = EXCLUDED.description,
		    status = EXCLUDED.status;

		INSERT INTO auctions (
		  id, room_id, item_id, status, is_narrating,
		  current_price_cents, current_winner_id,
		  start_price_cents, increment_cents, cap_price_cents,
		  start_at, end_at, version, seq, accepted_bid_count,
		  extend_count, rule_version, updated_at
		)
		VALUES (
		  'auc_live', 'room_main', 'item_live', 'ACTIVE', true,
		  35000, 'user_2',
		  10000, 5000, 60000,
		  now() - interval '1 minute', now() + interval '10 minutes',
		  1, 41, 1,
		  0, 1, now()
		);

		INSERT INTO auction_rules (
		  auction_id, rule_version, duration_seconds,
		  extend_window_seconds, extend_by_seconds, max_extend_count,
		  fat_finger_threshold_cents, deposit_bps,
		  deposit_floor_cents, deposit_cap_cents, frozen_at
		)
		VALUES (
		  'auc_live', 1, 600,
		  10, 10, 3,
		  100000, 1000,
		  5000, 50000, now()
		);

		INSERT INTO orders (
		  id, auction_id, winner_id, amount_cents, status,
		  deposit_cents, deposit_status, expire_at
		)
		VALUES (
		  'ord_pending', 'auc_live', 'user_1', 60000, 'ORDER_PENDING',
		  6000, 'HELD', now() + interval '15 minutes'
		);
	`)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return clearRedisSmokeKeys(ctx, rdb)
}

func clearRedisSmokeKeys(ctx context.Context, rdb *redis.Client) error {
	keys := []string{"auction:auc_live:events", "auction:auc_live:snapshot"}
	var cursor uint64
	for {
		matched, next, err := rdb.Scan(ctx, cursor, "ws_ticket:*", 100).Result()
		if err != nil {
			return err
		}
		keys = append(keys, matched...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return rdb.Del(ctx, keys...).Err()
}
