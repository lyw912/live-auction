package demo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func SeedP0Smoke(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE p0_demo_auctions ON COMMIT DROP AS
		SELECT id
		FROM auctions
		WHERE room_id IN ('room_main', 'room_side')
		   OR room_id IN (SELECT id FROM rooms WHERE host_id = 'host_1')
		   OR id IN ('auc_live', 'auc_side');

		CREATE TEMP TABLE p0_demo_items ON COMMIT DROP AS
		SELECT DISTINCT item_id AS id
		FROM auctions
		WHERE id IN (SELECT id FROM p0_demo_auctions)
		   OR room_id IN ('room_main', 'room_side');

		CREATE TEMP TABLE p0_demo_orders ON COMMIT DROP AS
		SELECT id
		FROM orders
		WHERE auction_id IN (SELECT id FROM p0_demo_auctions);

		CREATE TEMP TABLE p0_demo_jobs ON COMMIT DROP AS
		SELECT id
		FROM ai_generation_jobs
		WHERE auction_id IN (SELECT id FROM p0_demo_auctions)
		   OR room_id IN ('room_main', 'room_side');

		DELETE FROM scheduler_jobs
		WHERE target_id IN (SELECT id FROM p0_demo_auctions)
		   OR target_id IN (SELECT id FROM p0_demo_orders);
		DELETE FROM idempotency_records
		WHERE scope_id IN (SELECT id FROM p0_demo_auctions)
		   OR scope_id IN (SELECT id FROM p0_demo_orders);
		DELETE FROM max_bid_intents WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM bids WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM outbox_delivery
		WHERE outbox_id IN (SELECT id FROM outbox_events WHERE auction_id IN (SELECT id FROM p0_demo_auctions));
		DELETE FROM outbox_events WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM redis_engine_settlements WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM snapshot_rebuild_events WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM auction_events WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM auction_ai_settings WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM auction_highlight_assets WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM ai_generation_jobs WHERE id IN (SELECT id FROM p0_demo_jobs);
		DELETE FROM auction_risk_alerts WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM auction_system_messages WHERE auction_id IN (SELECT id FROM p0_demo_auctions) OR room_id IN ('room_main', 'room_side');
		DELETE FROM system_control_signals
		WHERE target_id IN (SELECT id FROM p0_demo_auctions)
		   OR target_id IN ('room_main', 'room_side');
		DELETE FROM user_activity_events
		WHERE auction_id IN (SELECT id FROM p0_demo_auctions)
		   OR room_id IN ('room_main', 'room_side');
		DELETE FROM system_anomaly_events
		WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM liveops_lucky_draw_entries WHERE room_id IN ('room_main', 'room_side');
		DELETE FROM liveops_team_choices WHERE room_id IN ('room_main', 'room_side');
		DELETE FROM liveops_task_progress WHERE room_id IN ('room_main', 'room_side');
		DELETE FROM liveops_campaigns WHERE room_id IN ('room_main', 'room_side');
		DELETE FROM payment_events
		WHERE order_id IN (SELECT id FROM p0_demo_orders);
		DELETE FROM orders WHERE id IN (SELECT id FROM p0_demo_orders);
		DELETE FROM auction_rules WHERE auction_id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM auctions WHERE id IN (SELECT id FROM p0_demo_auctions);
		DELETE FROM items WHERE id IN (SELECT id FROM p0_demo_items) AND id NOT IN ('item_live', 'item_side');
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
		  '天然翡翠A货平安扣吊坠',
		  '/demo/jade-pendant.jpg',
		  '天然A货翡翠平安扣，附GID证书，顺丰包邮，支持7天无理由。',
		  'READY'
		),
		(
		  'item_side',
		  '和田玉福牌吊坠',
		  '/demo/jade-pendant.jpg',
		  '温润和田玉福牌，附鉴定证书，支持7天无理由。',
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
		  35000, NULL,
		  35000, 5000, 60000,
		  now() - interval '1 minute', now() + interval '10 minutes',
		  1, 0, 0,
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

		DELETE FROM scheduler_jobs WHERE idempotency_key = 'end:auc_live';
		INSERT INTO scheduler_jobs (
		  id, job_type, target_type, target_id, idempotency_key, run_at, status, next_attempt_at
		)
		VALUES (
		  'job_end_auc_live', 'END_AUCTION', 'auction', 'auc_live', 'end:auc_live',
		  (SELECT end_at FROM auctions WHERE id = 'auc_live'), 'PENDING',
		  (SELECT end_at FROM auctions WHERE id = 'auc_live')
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
	auctionIDs := []string{"auc_live", "auc_side"}
	keys := []string{
		"auction:auc_live:events",
		"auction:auc_live:snapshot",
		"auction:auc_side:events",
		"auction:auc_side:snapshot",
		"presence:room:room_main",
		"presence:room:room_side",
	}
	for _, auctionID := range auctionIDs {
		keys = append(keys,
			"bid:{"+auctionID+"}:engine:state",
			"bid:{"+auctionID+"}:engine:pending",
			"bid:{"+auctionID+"}:engine:append-marker",
			"bid:{"+auctionID+"}:engine:append-stats",
			"bid:{"+auctionID+"}:engine:log",
			"bid:{"+auctionID+"}:engine:relay-cursor",
			"bid:{"+auctionID+"}:guard:projection",
		)
	}
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
	for _, auctionID := range auctionIDs {
		cursor = 0
		for {
			matched, next, err := rdb.Scan(ctx, cursor, "bid:{"+auctionID+"}:engine:idem:*", 100).Result()
			if err != nil {
				return err
			}
			keys = append(keys, matched...)
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	for _, roomID := range []string{"room_main", "room_side"} {
		cursor = 0
		for {
			matched, next, err := rdb.Scan(ctx, cursor, "presence:room:"+roomID+":conn:*", 100).Result()
			if err != nil {
				return err
			}
			keys = append(keys, matched...)
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return rdb.Del(ctx, keys...).Err()
}
