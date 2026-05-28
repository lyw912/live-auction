package invariant

import (
	"context"
	"fmt"
	"strings"
)

func (c *Checker) checkAuctionSeqMatchesEvents(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.seq, COALESCE(max(e.seq), 0) AS max_event_seq
			FROM auctions a
			LEFT JOIN auction_events e ON e.auction_id = a.id
			%s
			GROUP BY a.id, a.seq
		), violations AS (
			SELECT id AS auction_id, seq AS auction_seq, max_event_seq
			FROM scoped
			WHERE seq <> max_event_seq
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("auction_seq_matches_events", SeverityP0, "auctions.seq must equal the max auction_events.seq for the same auction.", countDetails(details), details), err
}

func (c *Checker) checkAuctionEventSeqContiguous(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id AS auction_id, max(e.seq) AS max_seq
			FROM auctions a
			JOIN auction_events e ON e.auction_id = a.id
			%s
			GROUP BY a.id
		), missing AS (
			SELECT s.auction_id, gs.seq AS missing_seq
			FROM scoped s
			CROSS JOIN LATERAL generate_series(1, s.max_seq) AS gs(seq)
			LEFT JOIN auction_events e ON e.auction_id = s.auction_id AND e.seq = gs.seq
			WHERE e.id IS NULL
		)
		SELECT *, count(*) OVER() AS total
		FROM missing
		ORDER BY auction_id, missing_seq
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("auction_event_seq_contiguous", SeverityP0, "auction_events must be contiguous from seq 1 through max seq; gaps require explicit DEAD/gap semantics, not missing DB truth.", countDetails(details), details), err
}

func (c *Checker) checkSingleTerminalEvent(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), terminal_counts AS (
			SELECT e.auction_id, count(*) AS terminal_events, array_agg(e.event_type ORDER BY e.seq) AS event_types
			FROM auction_events e
			JOIN scoped s ON s.id = e.auction_id
			WHERE e.event_type IN ('auction_sold','auction_ended','auction_cancelled')
			GROUP BY e.auction_id
			HAVING count(*) > 1
		)
		SELECT *, count(*) OVER() AS total
		FROM terminal_counts
		ORDER BY auction_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("single_terminal_event", SeverityP0, "An auction may have at most one terminal event: sold, ended, or cancelled.", countDetails(details), details), err
}

func (c *Checker) checkTerminalStatusMatchesEvent(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.status
			FROM auctions a
			%s
		), latest_terminal AS (
			SELECT DISTINCT ON (e.auction_id)
				e.auction_id, e.event_type, e.seq
			FROM auction_events e
			JOIN scoped s ON s.id = e.auction_id
			WHERE e.event_type IN ('auction_sold','auction_ended','auction_cancelled')
			ORDER BY e.auction_id, e.seq DESC
		), violations AS (
			SELECT s.id AS auction_id, s.status, lt.event_type, lt.seq
			FROM scoped s
			LEFT JOIN latest_terminal lt ON lt.auction_id = s.id
			WHERE (s.status = 'SOLD' AND lt.event_type IS DISTINCT FROM 'auction_sold')
			   OR (s.status = 'ENDED' AND lt.event_type IS DISTINCT FROM 'auction_ended')
			   OR (s.status = 'CANCELLED' AND lt.event_type IS DISTINCT FROM 'auction_cancelled')
			   OR (s.status NOT IN ('SOLD','ENDED','CANCELLED') AND lt.event_type IS NOT NULL)
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("terminal_status_matches_event", SeverityP0, "Terminal auction status must be backed by exactly the matching terminal event and non-terminal auctions cannot have a terminal event.", countDetails(details), details), err
}

func (c *Checker) checkAcceptedBidCountMatches(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.accepted_bid_count
			FROM auctions a
			%s
		), counts AS (
			SELECT s.id AS auction_id, s.accepted_bid_count, count(b.id) AS accepted_bids
			FROM scoped s
			LEFT JOIN bids b ON b.auction_id = s.id AND b.status = 'ACCEPTED'
			GROUP BY s.id, s.accepted_bid_count
		)
		SELECT *, count(*) OVER() AS total
		FROM counts
		WHERE accepted_bid_count <> accepted_bids
		ORDER BY auction_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("accepted_bid_count_matches", SeverityP0, "auctions.accepted_bid_count must equal accepted bids recorded in bids.", countDetails(details), details), err
}

func (c *Checker) checkWinnerMatchesLatestAcceptedBid(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.status, a.current_winner_id, a.current_price_cents, a.start_price_cents, a.accepted_bid_count
			FROM auctions a
			%s
		), latest_bid AS (
			SELECT DISTINCT ON (b.auction_id)
				b.auction_id, b.id AS bid_id, b.user_id, b.amount_cents, b.seq
			FROM bids b
			JOIN scoped s ON s.id = b.auction_id
			WHERE b.status = 'ACCEPTED'
			ORDER BY b.auction_id, b.seq DESC, b.created_at DESC, b.id DESC
		), violations AS (
			SELECT s.id AS auction_id, s.status, s.current_winner_id, s.current_price_cents, s.start_price_cents,
			       s.accepted_bid_count, lb.bid_id, lb.user_id AS latest_bid_user_id,
			       lb.amount_cents AS latest_bid_amount_cents, lb.seq AS latest_bid_seq
			FROM scoped s
			LEFT JOIN latest_bid lb ON lb.auction_id = s.id
			WHERE (s.accepted_bid_count = 0 AND (s.current_winner_id IS NOT NULL OR s.current_price_cents <> s.start_price_cents))
			   OR (s.accepted_bid_count > 0 AND lb.bid_id IS NULL)
			   OR (s.accepted_bid_count > 0 AND (s.current_winner_id IS DISTINCT FROM lb.user_id OR s.current_price_cents <> lb.amount_cents))
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("winner_price_matches_latest_accepted_bid", SeverityP0, "Current winner and price must equal the latest accepted bid, or be empty before any accepted bid.", countDetails(details), details), err
}

func (c *Checker) checkSoldOrderMatchesAuction(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.status, a.current_winner_id, a.current_price_cents
			FROM auctions a
			%s
		), violations AS (
			SELECT s.id AS auction_id, s.current_winner_id, s.current_price_cents,
			       count(o.id) AS order_count,
			       max(o.id) AS order_id,
			       max(o.winner_id) AS order_winner_id,
			       max(o.amount_cents) AS order_amount_cents
			FROM scoped s
			LEFT JOIN orders o ON o.auction_id = s.id
			WHERE s.status = 'SOLD'
			GROUP BY s.id, s.current_winner_id, s.current_price_cents
			HAVING count(o.id) <> 1
			    OR s.current_winner_id IS NULL
			    OR max(o.winner_id) IS DISTINCT FROM s.current_winner_id
			    OR max(o.amount_cents) <> s.current_price_cents
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("sold_order_matches_auction", SeverityP0, "A SOLD auction must have exactly one order matching current_winner_id and current_price_cents.", countDetails(details), details), err
}

func (c *Checker) checkNonSoldOrderLeak(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.status
			FROM auctions a
			%s
		)
		SELECT s.id AS auction_id, s.status, o.id AS order_id, o.status AS order_status, count(*) OVER() AS total
		FROM scoped s
		JOIN orders o ON o.auction_id = s.id
		WHERE s.status <> 'SOLD'
		ORDER BY s.id, o.id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("non_sold_order_leak", SeverityP0, "Orders are only valid for SOLD auctions.", countDetails(details), details), err
}

func (c *Checker) checkOutboxCoverage(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), violations AS (
			SELECT e.auction_id, e.seq, e.event_type
			FROM auction_events e
			JOIN scoped s ON s.id = e.auction_id
			LEFT JOIN outbox_events o
			  ON o.aggregate_type = 'auction'
			 AND o.aggregate_id = e.auction_id
			 AND o.auction_id = e.auction_id
			 AND o.seq = e.seq
			 AND o.event_type = e.event_type
			WHERE o.id IS NULL
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id, seq
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_covers_auction_events", SeverityP0, "Every auction_event must have a matching auction outbox_event with the same auction_id, seq, and event_type.", countDetails(details), details), err
}

func (c *Checker) checkOutboxNoExtraAuctionEvents(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), violations AS (
			SELECT o.id AS outbox_id, o.auction_id, o.seq, o.event_type
			FROM outbox_events o
			JOIN scoped s ON s.id = o.auction_id
			LEFT JOIN auction_events e
			  ON e.auction_id = o.auction_id
			 AND e.seq = o.seq
			 AND e.event_type = o.event_type
			WHERE o.aggregate_type = 'auction'
			  AND o.seq IS NOT NULL
			  AND e.id IS NULL
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id, seq, outbox_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_has_no_extra_auction_events", SeverityP0, "Auction outbox_events must correspond to an auction_event with the same auction_id, seq, and event_type.", countDetails(details), details), err
}

func (c *Checker) checkOutboxPayloadMatchesAuctionEvent(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		)
		SELECT o.id AS outbox_id, o.auction_id, o.seq, o.event_type, count(*) OVER() AS total
		FROM outbox_events o
		JOIN scoped s ON s.id = o.auction_id
		JOIN auction_events e
		  ON e.auction_id = o.auction_id
		 AND e.seq = o.seq
		 AND e.event_type = o.event_type
		WHERE o.aggregate_type = 'auction'
		  AND o.payload_json IS DISTINCT FROM e.payload_json
		ORDER BY o.auction_id, o.seq, o.id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_payload_matches_auction_event", SeverityP0, "Outbox payload must match the immutable auction_event payload for the same auction_id, seq, and event_type.", countDetails(details), details), err
}

func (c *Checker) checkOutboxDeliveryCoverage(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		)
		SELECT o.id AS outbox_id, o.auction_id, o.seq, o.event_type, count(*) OVER() AS total
		FROM outbox_events o
		JOIN scoped s ON s.id = o.auction_id
		LEFT JOIN outbox_delivery d ON d.outbox_id = o.id
		WHERE d.outbox_id IS NULL
		ORDER BY o.auction_id, o.seq, o.id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_delivery_exists", SeverityP0, "Every outbox_event must have exactly one outbox_delivery row.", countDetails(details), details), err
}

func (c *Checker) checkOutboxDeliveryMirror(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		)
		SELECT o.id AS outbox_id, o.auction_id AS event_auction_id, o.seq AS event_seq,
		       d.auction_id AS delivery_auction_id, d.auction_seq AS delivery_auction_seq,
		       count(*) OVER() AS total
		FROM outbox_events o
		JOIN scoped s ON s.id = o.auction_id
		JOIN outbox_delivery d ON d.outbox_id = o.id
		WHERE d.auction_id IS DISTINCT FROM o.auction_id
		   OR d.auction_seq IS DISTINCT FROM o.seq
		ORDER BY o.auction_id, o.seq, o.id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_delivery_mirrors_event_fields", SeverityP1, "Denormalized outbox_delivery auction identity fields must mirror outbox_events for claim ordering and diagnostics.", countDetails(details), details), err
}

func (c *Checker) checkOutboxHeadOfLine(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), ordered AS (
			SELECT o.auction_id, o.seq, o.id AS outbox_id, d.status
			FROM outbox_events o
			JOIN scoped s ON s.id = o.auction_id
			JOIN outbox_delivery d ON d.outbox_id = o.id
			WHERE o.seq IS NOT NULL
		), violations AS (
			SELECT high.auction_id,
			       low.seq AS lower_seq,
			       low.outbox_id AS lower_outbox_id,
			       low.status AS lower_status,
			       high.seq AS published_seq,
			       high.outbox_id AS published_outbox_id
			FROM ordered high
			JOIN ordered low ON low.auction_id = high.auction_id AND low.seq < high.seq
			WHERE high.status = 'PUBLISHED'
			  AND low.status IN ('PENDING','PUBLISHING','FAILED')
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id, lower_seq, published_seq
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_same_auction_head_of_line", SeverityP0, "A higher same-auction seq cannot be PUBLISHED while a lower seq is still pending/publishing/failed; DEAD is the explicit gap exception.", countDetails(details), details), err
}

func (c *Checker) checkOutboxPayloadHash(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		)
		SELECT o.id AS outbox_id, o.auction_id, o.seq, o.event_type, o.payload_sha256,
		       encode(digest(convert_to(o.payload_json::text, 'UTF8'), 'sha256'), 'hex') AS expected_sha256,
		       count(*) OVER() AS total
		FROM outbox_events o
		JOIN scoped s ON s.id = o.auction_id
		WHERE o.payload_sha256 IS DISTINCT FROM encode(digest(convert_to(o.payload_json::text, 'UTF8'), 'sha256'), 'hex')
		ORDER BY o.auction_id, o.seq, o.id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_payload_sha256_valid", SeverityP1, "outbox_events.payload_sha256 must match the stored payload_json digest for auditability.", countDetails(details), details), err
}

func (c *Checker) checkPublishedDeliveryFields(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		)
		SELECT o.id AS outbox_id, o.auction_id, o.seq, d.status, d.published_at,
		       d.locked_by, d.locked_until, d.last_error, count(*) OVER() AS total
		FROM outbox_events o
		JOIN scoped s ON s.id = o.auction_id
		JOIN outbox_delivery d ON d.outbox_id = o.id
		WHERE (d.status = 'PUBLISHED' AND d.published_at IS NULL)
		   OR (d.status = 'PUBLISHED' AND (d.locked_by IS NOT NULL OR d.locked_until IS NOT NULL))
		   OR (d.status <> 'PUBLISHED' AND d.published_at IS NOT NULL)
		ORDER BY o.auction_id, o.seq, o.id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("outbox_delivery_status_fields", SeverityP1, "outbox_delivery status, publication timestamp, and lock fields must be internally consistent.", countDetails(details), details), err
}

func (c *Checker) checkDeadOutboxAnomaly(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		)
		SELECT o.id AS outbox_id, o.auction_id, o.seq, d.status, d.last_error_class,
		       count(*) OVER() AS total
		FROM outbox_events o
		JOIN scoped s ON s.id = o.auction_id
		JOIN outbox_delivery d ON d.outbox_id = o.id
		WHERE d.status = 'DEAD'
		  AND NOT EXISTS (
		    SELECT 1
		    FROM system_anomaly_events sae
		    WHERE sae.auction_id = o.auction_id
		      AND sae.type = 'OUTBOX_DEAD_LETTER'
		      AND sae.payload_json->>'outbox_id' = o.id::text
		  )
		ORDER BY o.auction_id, o.seq, o.id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("dead_outbox_has_anomaly", SeverityP1, "DEAD outbox rows must have a matching OUTBOX_DEAD_LETTER anomaly for operator recovery and gap explanation.", countDetails(details), details), err
}

func (c *Checker) checkBidIdempotency(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), violations AS (
			SELECT ir.scope_id AS auction_id, ir.user_id, ir.idempotency_key,
			       ir.status, ir.http_status, ir.result_code, b.id AS bid_id,
			       b.amount_cents, b.status AS bid_status, b.request_hash AS bid_request_hash,
			       encode(digest(convert_to('bid:v1|' || ir.scope_id || '|' || ir.user_id || '|' || ir.idempotency_key || '|' || COALESCE(b.amount_cents::text, ''), 'UTF8'), 'sha256'), 'hex') AS expected_hash,
			       CASE
			       	 WHEN ir.status = 'COMPLETED' AND (ir.http_status IS NULL OR ir.result_code IS NULL OR ir.response_json IS NULL OR ir.completed_at IS NULL) THEN 'completed_missing_response'
			       	 WHEN ir.status = 'COMPLETED' AND ir.result_code <> 'FAT_FINGER_CONFIRM_REQUIRED' AND b.id IS NULL THEN 'completed_without_bid'
			       	 WHEN b.id IS NOT NULL AND ir.request_hash <> b.request_hash THEN 'idempotency_bid_hash_mismatch'
			       	 WHEN b.id IS NOT NULL AND ir.request_hash <> encode(digest(convert_to('bid:v1|' || ir.scope_id || '|' || ir.user_id || '|' || ir.idempotency_key || '|' || b.amount_cents::text, 'UTF8'), 'sha256'), 'hex') THEN 'computed_hash_mismatch'
			       	 ELSE 'unknown'
			       END AS reason
			FROM idempotency_records ir
			JOIN scoped s ON s.id = ir.scope_id
			LEFT JOIN bids b
			  ON b.auction_id = ir.scope_id
			 AND b.user_id = ir.user_id
			 AND b.client_bid_id = ir.idempotency_key
			WHERE ir.scope_type = 'bid'
			  AND (
			    (ir.status = 'COMPLETED' AND (ir.http_status IS NULL OR ir.result_code IS NULL OR ir.response_json IS NULL OR ir.completed_at IS NULL))
			    OR (ir.status = 'COMPLETED' AND ir.result_code <> 'FAT_FINGER_CONFIRM_REQUIRED' AND b.id IS NULL)
			    OR (b.id IS NOT NULL AND ir.request_hash <> b.request_hash)
			    OR (b.id IS NOT NULL AND ir.request_hash <> encode(digest(convert_to('bid:v1|' || ir.scope_id || '|' || ir.user_id || '|' || ir.idempotency_key || '|' || b.amount_cents::text, 'UTF8'), 'sha256'), 'hex'))
			  )
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id, user_id, idempotency_key
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("bid_idempotency_consistent", SeverityP0, "Completed bid idempotency must have replay data and align with the bid row/hash, except fat-finger confirm-required preflight.", countDetails(details), details), err
}

func (c *Checker) checkPaymentIdempotency(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	missingOrderPredicate := "o.id IS NULL"
	if scopeSQL != "" {
		missingOrderPredicate = "FALSE"
	}
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), violations AS (
			SELECT o.auction_id, ir.scope_id AS order_id, ir.user_id, ir.idempotency_key,
			       ir.status, ir.http_status, ir.result_code, o.winner_id, o.status AS order_status,
			       CASE
			       	 WHEN o.id IS NULL THEN 'missing_order'
			       	 WHEN o.winner_id <> ir.user_id THEN 'payer_not_winner'
			       	 WHEN ir.status = 'COMPLETED' AND (ir.http_status IS NULL OR ir.result_code IS NULL OR ir.response_json IS NULL OR ir.completed_at IS NULL) THEN 'completed_missing_response'
			       	 WHEN ir.request_hash <> encode(digest(convert_to('payment:v1|' || ir.scope_id || '|' || ir.user_id || '|' || ir.idempotency_key, 'UTF8'), 'sha256'), 'hex') THEN 'computed_hash_mismatch'
			       	 ELSE 'unknown'
			       END AS reason
			FROM idempotency_records ir
			LEFT JOIN orders o ON o.id = ir.scope_id
			LEFT JOIN scoped s ON s.id = o.auction_id
			WHERE ir.scope_type = 'payment'
			  AND (%s OR s.id IS NOT NULL)
			  AND (
			    o.id IS NULL
			    OR o.winner_id <> ir.user_id
			    OR (ir.status = 'COMPLETED' AND (ir.http_status IS NULL OR ir.result_code IS NULL OR ir.response_json IS NULL OR ir.completed_at IS NULL))
			    OR ir.request_hash <> encode(digest(convert_to('payment:v1|' || ir.scope_id || '|' || ir.user_id || '|' || ir.idempotency_key, 'UTF8'), 'sha256'), 'hex')
			  )
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id, order_id, user_id, idempotency_key
		LIMIT %s
	`, scopeSQL, missingOrderPredicate, limit), queryArgs...)
	return failOrPass("payment_idempotency_consistent", SeverityP0, "Payment idempotency must replay completed payment and be scoped to the order winner with the expected hash. Missing-order payment records are checked in unscoped full-database mode.", countDetails(details), details), err
}

func (c *Checker) checkRedisEngineSettlementContiguous(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), ranges AS (
			SELECT s.auction_id, s.engine_epoch, max(s.engine_seq) AS max_seq
			FROM redis_engine_settlements s
			JOIN scoped a ON a.id = s.auction_id
			WHERE s.status = 'SETTLED'
			GROUP BY s.auction_id, s.engine_epoch
		), missing AS (
			SELECT r.auction_id, r.engine_epoch, gs.seq AS missing_engine_seq
			FROM ranges r
			CROSS JOIN LATERAL generate_series(1, r.max_seq) AS gs(seq)
			LEFT JOIN redis_engine_settlements s
			  ON s.auction_id = r.auction_id
			 AND s.engine_epoch = r.engine_epoch
			 AND s.engine_seq = gs.seq
			 AND s.status = 'SETTLED'
			WHERE s.id IS NULL
		)
		SELECT *, count(*) OVER() AS total
		FROM missing
		ORDER BY auction_id, engine_epoch, missing_engine_seq
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("redis_engine_settlement_seq_contiguous", SeverityP0, "Redis/Kafka engine settlement rows must be contiguous per auction epoch before they are treated as PostgreSQL truth.", countDetails(details), details), err
}

func (c *Checker) checkRedisEngineSeqMatchesSettlement(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.engine_epoch, a.engine_seq
			FROM auctions a
			%s
		), settled AS (
			SELECT s.auction_id, s.engine_epoch, COALESCE(max(s.engine_seq), 0) AS max_settled_seq
			FROM redis_engine_settlements s
			JOIN scoped a ON a.id = s.auction_id AND a.engine_epoch = s.engine_epoch
			WHERE s.status = 'SETTLED'
			GROUP BY s.auction_id, s.engine_epoch
		), violations AS (
			SELECT a.id AS auction_id, a.engine_epoch, a.engine_seq, COALESCE(s.max_settled_seq, 0) AS max_settled_seq
			FROM scoped a
			LEFT JOIN settled s ON s.auction_id = a.id AND s.engine_epoch = a.engine_epoch
			WHERE a.engine_seq > 0 AND a.engine_seq <> COALESCE(s.max_settled_seq, 0)
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("redis_engine_seq_matches_settlement", SeverityP0, "auctions.engine_seq must equal the latest settled Redis/Kafka engine ledger seq for the current engine epoch.", countDetails(details), details), err
}

func (c *Checker) checkRedisEngineLedgerHealthy(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		)
		SELECT s.auction_id, s.stream_id, s.ledger_source, s.ledger_topic, s.ledger_partition,
		       s.ledger_offset, s.engine_epoch, s.engine_seq, s.status, s.attempts,
		       s.last_error, s.dlq_topic, s.dlq_at, count(*) OVER() AS total
		FROM redis_engine_settlements s
		JOIN scoped a ON a.id = s.auction_id
		WHERE s.status = 'FAILED'
		   OR s.dlq_at IS NOT NULL
		   OR s.attempts > 3
		ORDER BY s.auction_id, s.engine_epoch, s.engine_seq
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("redis_engine_ledger_no_failed_or_dlq", SeverityP0, "Kafka-backed Redis engine ledger must not contain failed, dead-lettered, or over-retried settlement rows without operator reconciliation.", countDetails(details), details), err
}

func (c *Checker) checkRedisEngineEventCoverage(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id
			FROM auctions a
			%s
		), accepted_settlements AS (
			SELECT s.auction_id, s.engine_epoch, s.engine_seq, s.result
			FROM redis_engine_settlements s
			JOIN scoped a ON a.id = s.auction_id
			WHERE s.status = 'SETTLED'
			  AND s.result IN ('ENGINE_ACCEPTED','ENGINE_SOLD')
		), violations AS (
			SELECT s.auction_id, s.engine_epoch, s.engine_seq, s.result,
			       b.id AS bid_id, e.id AS event_id
			FROM accepted_settlements s
			LEFT JOIN bids b
			  ON b.auction_id = s.auction_id
			 AND b.engine_epoch = s.engine_epoch
			 AND b.engine_seq = s.engine_seq
			 AND b.status = 'ACCEPTED'
			LEFT JOIN auction_events e
			  ON e.auction_id = s.auction_id
			 AND e.engine_epoch = s.engine_epoch
			 AND e.engine_seq = s.engine_seq
			 AND e.event_type IN ('bid_accepted','auction_sold')
			WHERE b.id IS NULL OR e.id IS NULL
		)
		SELECT *, count(*) OVER() AS total
		FROM violations
		ORDER BY auction_id, engine_epoch, engine_seq
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("redis_engine_accepted_settlement_has_bid_event", SeverityP0, "Every accepted/sold Redis engine settlement must have matching bid and auction_event rows with the same engine epoch/seq.", countDetails(details), details), err
}

func (c *Checker) checkRoomIsolation(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.id, a.room_id
			FROM auctions a
			%s
		), payload_room_refs AS (
			SELECT e.auction_id, s.room_id AS auction_room_id, e.seq, e.event_type,
			       e.payload_json->>'room_id' AS payload_room_id
			FROM auction_events e
			JOIN scoped s ON s.id = e.auction_id
			WHERE e.payload_json ? 'room_id'
		)
		SELECT *, count(*) OVER() AS total
		FROM payload_room_refs
		WHERE payload_room_id IS DISTINCT FROM auction_room_id
		ORDER BY auction_id, seq
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("no_cross_room_payload_leak", SeverityP0, "Any room_id embedded in auction event payloads must match the auction room.", countDetails(details), details), err
}

func (c *Checker) checkRoomActiveUniqueness(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.*
			FROM auctions a
			%s
		), counts AS (
			SELECT room_id, count(*) AS active_count, array_agg(id ORDER BY id) AS auction_ids
			FROM scoped
			WHERE status = 'ACTIVE'
			GROUP BY room_id
			HAVING count(*) > 1
		)
		SELECT *, count(*) OVER() AS total
		FROM counts
		ORDER BY room_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("one_active_auction_per_room", SeverityP0, "Each room may have at most one ACTIVE auction.", countDetails(details), details), err
}

func (c *Checker) checkRoomNarratingUniqueness(ctx context.Context, scopeSQL string, args []any, maxDetails int) (CheckResult, error) {
	queryArgs, limit := limitArg(args, maxDetails)
	details, err := c.queryDetails(ctx, fmt.Sprintf(`
		WITH scoped AS (
			SELECT a.*
			FROM auctions a
			%s
		), counts AS (
			SELECT room_id, count(*) AS narrating_count, array_agg(id ORDER BY id) AS auction_ids
			FROM scoped
			WHERE is_narrating
			GROUP BY room_id
			HAVING count(*) > 1
		)
		SELECT *, count(*) OVER() AS total
		FROM counts
		ORDER BY room_id
		LIMIT %s
	`, scopeSQL, limit), queryArgs...)
	return failOrPass("one_narrating_auction_per_room", SeverityP0, "Each room may have at most one narrating auction.", countDetails(details), details), err
}

func hasAnyColumn(details []ViolationDetail, column string) bool {
	for _, detail := range details {
		if _, ok := detail[column]; ok {
			return true
		}
	}
	return false
}

func joinReasons(details []ViolationDetail) string {
	reasons := make([]string, 0, len(details))
	for _, detail := range details {
		if reason, ok := detail["reason"].(string); ok {
			reasons = append(reasons, reason)
		}
	}
	return strings.Join(reasons, ",")
}
