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
		WHERE target_id IN ('auc_live', 'auc_side')
		   OR target_id IN (SELECT id FROM orders WHERE auction_id IN ('auc_live', 'auc_side'));
		DELETE FROM idempotency_records
		WHERE scope_id IN ('auc_live', 'auc_side')
		   OR scope_id IN (SELECT id FROM orders WHERE auction_id IN ('auc_live', 'auc_side'));
		DELETE FROM bids WHERE auction_id IN ('auc_live', 'auc_side');
		DELETE FROM outbox_delivery
		WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id IN ('auc_live', 'auc_side'));
		DELETE FROM outbox_events WHERE auction_id IN ('auc_live', 'auc_side');
		DELETE FROM auction_events WHERE auction_id IN ('auc_live', 'auc_side');
		DELETE FROM payment_events
		WHERE order_id IN (SELECT id FROM orders WHERE auction_id IN ('auc_live', 'auc_side'));
		DELETE FROM orders WHERE auction_id IN ('auc_live', 'auc_side');
		DELETE FROM auction_rules WHERE auction_id IN ('auc_live', 'auc_side');
		DELETE FROM auctions WHERE id IN ('auc_live', 'auc_side');
		DELETE FROM chat_messages WHERE room_id IN ('room_main', 'room_side');

		INSERT INTO users (id, role, display_name, city)
		VALUES
		  ('host_1', 'host', 'Demo Host', 'Hangzhou'),
		  ('user_1', 'user', 'Demo User', 'Shanghai'),
		  ('user_2', 'user', 'Prior Leader', 'Beijing'),
		  ('user_3', 'user', 'Smoke Bidder', 'Shenzhen')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO rooms (id, host_id, status)
		VALUES
		  ('room_main', 'host_1', 'OPEN'),
		  ('room_side', 'host_1', 'OPEN')
		ON CONFLICT (id) DO UPDATE SET host_id = EXCLUDED.host_id, status = EXCLUDED.status;

		INSERT INTO room_memberships (room_id, user_id, role, status)
		VALUES
		  ('room_main', 'host_1', 'host', 'ACTIVE'),
		  ('room_main', 'user_1', 'viewer', 'ACTIVE'),
		  ('room_main', 'user_2', 'viewer', 'ACTIVE'),
		  ('room_main', 'user_3', 'viewer', 'ACTIVE'),
		  ('room_side', 'host_1', 'host', 'ACTIVE'),
		  ('room_side', 'user_1', 'viewer', 'ACTIVE')
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, status = EXCLUDED.status, left_at = NULL;

		INSERT INTO items (id, title, image_url, description, status)
		VALUES
		(
		  'item_live',
		  'P0 Live Smoke Item',
		  '/demo/ceramic-tea-cup.jpg',
		  'P0 live backend smoke item.',
		  'READY'
		),
		(
		  'item_side',
		  'Side Room Smoke Item',
		  '/demo/ceramic-tea-cup.jpg',
		  'Second room smoke item.',
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
		  10000, 1000,
		  5000, 50000, now()
		);

		INSERT INTO auctions (
		  id, room_id, item_id, status, is_narrating,
		  current_price_cents, current_winner_id,
		  start_price_cents, increment_cents, cap_price_cents,
		  start_at, end_at, version, seq, accepted_bid_count,
		  extend_count, rule_version, updated_at
		)
		VALUES (
		  'auc_side', 'room_side', 'item_side', 'DRAFT', false,
		  20000, NULL,
		  20000, 10000, 80000,
		  NULL, NULL,
		  1, 1, 0,
		  0, 1, now()
		);

		INSERT INTO auction_rules (
		  auction_id, rule_version, duration_seconds,
		  extend_window_seconds, extend_by_seconds, max_extend_count,
		  fat_finger_threshold_cents, deposit_bps,
		  deposit_floor_cents, deposit_cap_cents, frozen_at
		)
		VALUES (
		  'auc_side', 1, 600,
		  10, 10, 3,
		  10000, 1000,
		  5000, 50000, NULL
		);

		INSERT INTO chat_messages (room_id, user_id, client_msg_id, body)
		VALUES
		  ('room_main', 'user_2', 'seed_chat_1', '这件拍品状态不错'),
		  ('room_main', 'user_3', 'seed_chat_2', '最后十秒再看'),
		  ('room_side', 'user_1', 'seed_side_chat_1', '侧房间独立弹幕')
		ON CONFLICT DO NOTHING;
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
