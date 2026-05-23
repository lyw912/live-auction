package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	DriftSnapshotMissing       = "SNAPSHOT_MISSING"
	DriftSnapshotInvalidJSON   = "SNAPSHOT_INVALID_JSON"
	DriftSnapshotSeqDrift      = "SNAPSHOT_SEQ_DRIFT"
	DriftSnapshotFieldDrift    = "SNAPSHOT_FIELD_DRIFT"
	DriftHistoryMissing        = "HISTORY_MISSING"
	DriftHistoryInvalidJSON    = "HISTORY_INVALID_JSON"
	DriftHistoryGap            = "HISTORY_GAP"
	DriftHistoryLastSeqDrift   = "HISTORY_LAST_SEQ_DRIFT"
	DriftDBEventSeqDrift       = "DB_EVENT_SEQ_DRIFT"
	AnomalyReconciliationDrift = "REDIS_DB_RECONCILIATION_DRIFT"
)

type Checker struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

type Options struct {
	Limit          int
	AuctionIDs     []string
	WriteAnomalies bool
}

type Report struct {
	CheckedAt       time.Time        `json:"checked_at"`
	AuctionsChecked int              `json:"auctions_checked"`
	DriftCount      int              `json:"drift_count"`
	Results         []AuctionResult  `json:"results"`
	Drifts          []Drift          `json:"drifts"`
	Anomalies       []WrittenAnomaly `json:"anomalies,omitempty"`
}

type AuctionResult struct {
	AuctionID          string   `json:"auction_id"`
	RoomID             string   `json:"room_id"`
	Status             string   `json:"status"`
	DBSeq              int64    `json:"db_seq"`
	DBMaxEventSeq      int64    `json:"db_max_event_seq"`
	RedisSnapshotSeq   *int64   `json:"redis_snapshot_seq,omitempty"`
	RedisSnapshotShape string   `json:"redis_snapshot_shape,omitempty"`
	RedisHistoryFirst  *int64   `json:"redis_history_first_seq,omitempty"`
	RedisHistoryLast   *int64   `json:"redis_history_last_seq,omitempty"`
	RedisHistoryCount  int      `json:"redis_history_count"`
	Drifts             []string `json:"drifts,omitempty"`
}

type Drift struct {
	AuctionID string         `json:"auction_id"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}

type WrittenAnomaly struct {
	ID        int64  `json:"id"`
	AuctionID string `json:"auction_id"`
	Type      string `json:"type"`
}

type dbAuction struct {
	id                string
	roomID            string
	status            string
	currentPriceCents int64
	currentWinnerID   *string
	seq               int64
	acceptedBidCount  int64
	extendCount       int64
	maxEventSeq       int64
}

func NewChecker(db *pgxpool.Pool, redisClient *redis.Client) *Checker {
	return &Checker{db: db, redis: redisClient}
}

func (c *Checker) Check(ctx context.Context, opts Options) (Report, error) {
	if opts.Limit <= 0 || opts.Limit > 500 {
		opts.Limit = 100
	}

	auctions, err := c.loadAuctions(ctx, opts)
	if err != nil {
		return Report{}, err
	}
	if err := ensureRequestedAuctionsFound(opts.AuctionIDs, auctions); err != nil {
		return Report{}, err
	}

	report := Report{
		CheckedAt:       time.Now().UTC(),
		AuctionsChecked: len(auctions),
	}
	for _, auction := range auctions {
		result, drifts := c.checkAuction(ctx, auction)
		report.Results = append(report.Results, result)
		report.Drifts = append(report.Drifts, drifts...)
	}
	report.DriftCount = len(report.Drifts)

	if opts.WriteAnomalies && report.DriftCount > 0 {
		written, err := c.writeAnomalies(ctx, report.Drifts)
		if err != nil {
			return Report{}, err
		}
		report.Anomalies = written
	}
	return report, nil
}

func (c *Checker) loadAuctions(ctx context.Context, opts Options) ([]dbAuction, error) {
	if len(opts.AuctionIDs) > 0 {
		rows, err := c.db.Query(ctx, `
			SELECT a.id, a.room_id, a.status, a.current_price_cents, a.current_winner_id,
			       a.seq, a.accepted_bid_count, a.extend_count, COALESCE(max(ev.seq), 0)::bigint
			FROM auctions a
			LEFT JOIN auction_events ev ON ev.auction_id = a.id
			WHERE a.id = ANY($1)
			GROUP BY a.id
			ORDER BY a.updated_at DESC
		`, opts.AuctionIDs)
		if err != nil {
			return nil, err
		}
		return scanAuctions(rows)
	}

	rows, err := c.db.Query(ctx, `
		SELECT a.id, a.room_id, a.status, a.current_price_cents, a.current_winner_id,
		       a.seq, a.accepted_bid_count, a.extend_count, COALESCE(max(ev.seq), 0)::bigint
		FROM auctions a
		LEFT JOIN auction_events ev ON ev.auction_id = a.id
		WHERE a.seq > 0
		GROUP BY a.id
		ORDER BY a.updated_at DESC
		LIMIT $1
	`, opts.Limit)
	if err != nil {
		return nil, err
	}
	return scanAuctions(rows)
}

func scanAuctions(rows pgx.Rows) ([]dbAuction, error) {
	defer rows.Close()
	var out []dbAuction
	for rows.Next() {
		var row dbAuction
		if err := rows.Scan(
			&row.id,
			&row.roomID,
			&row.status,
			&row.currentPriceCents,
			&row.currentWinnerID,
			&row.seq,
			&row.acceptedBidCount,
			&row.extendCount,
			&row.maxEventSeq,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (c *Checker) checkAuction(ctx context.Context, auction dbAuction) (AuctionResult, []Drift) {
	result := AuctionResult{
		AuctionID:     auction.id,
		RoomID:        auction.roomID,
		Status:        auction.status,
		DBSeq:         auction.seq,
		DBMaxEventSeq: auction.maxEventSeq,
	}
	var drifts []Drift
	add := func(d Drift) {
		drifts = append(drifts, d)
		result.Drifts = append(result.Drifts, d.Type)
	}

	if auction.seq != auction.maxEventSeq {
		add(Drift{
			AuctionID: auction.id,
			Type:      DriftDBEventSeqDrift,
			Severity:  "HIGH",
			Message:   "auction row seq does not match max auction_events seq",
			Details: map[string]any{
				"auction_seq":      auction.seq,
				"max_event_seq":    auction.maxEventSeq,
				"source_of_truth":  "postgres",
				"checked_relation": "auctions_vs_auction_events",
			},
		})
	}

	snapshot, err := c.redis.Get(ctx, snapshotKey(auction.id)).Bytes()
	if err == redis.Nil {
		add(Drift{
			AuctionID: auction.id,
			Type:      DriftSnapshotMissing,
			Severity:  "MED",
			Message:   "redis snapshot key is missing",
			Details: map[string]any{
				"key":             snapshotKey(auction.id),
				"source_of_truth": "postgres",
			},
		})
	} else if err != nil {
		add(Drift{
			AuctionID: auction.id,
			Type:      DriftSnapshotMissing,
			Severity:  "HIGH",
			Message:   "redis snapshot read failed",
			Details: map[string]any{
				"key":   snapshotKey(auction.id),
				"error": err.Error(),
			},
		})
	} else {
		for _, drift := range validateSnapshot(auction, snapshot, &result) {
			add(drift)
		}
	}

	values, err := c.redis.LRange(ctx, eventsKey(auction.id), 0, -1).Result()
	if err == redis.Nil || (err == nil && len(values) == 0) {
		add(Drift{
			AuctionID: auction.id,
			Type:      DriftHistoryMissing,
			Severity:  "MED",
			Message:   "redis history key is missing or empty",
			Details: map[string]any{
				"key":             eventsKey(auction.id),
				"source_of_truth": "postgres",
			},
		})
	} else if err != nil {
		add(Drift{
			AuctionID: auction.id,
			Type:      DriftHistoryMissing,
			Severity:  "HIGH",
			Message:   "redis history read failed",
			Details: map[string]any{
				"key":   eventsKey(auction.id),
				"error": err.Error(),
			},
		})
	} else {
		for _, drift := range validateHistory(auction, values, &result) {
			add(drift)
		}
	}

	return result, drifts
}

func validateSnapshot(auction dbAuction, data []byte, result *AuctionResult) []Drift {
	doc, err := decodeObject(data)
	if err != nil {
		return []Drift{{
			AuctionID: auction.id,
			Type:      DriftSnapshotInvalidJSON,
			Severity:  "HIGH",
			Message:   "redis snapshot is not valid JSON",
			Details:   map[string]any{"error": err.Error()},
		}}
	}

	seq, ok := int64Field(doc, "seq")
	if ok {
		result.RedisSnapshotSeq = &seq
	}
	eventType, _ := stringField(doc, "event_type")
	source, _ := stringField(doc, "source")
	result.RedisSnapshotShape = snapshotShape(eventType, source)

	var drifts []Drift
	if !ok || seq != auction.seq {
		drifts = append(drifts, Drift{
			AuctionID: auction.id,
			Type:      DriftSnapshotSeqDrift,
			Severity:  "HIGH",
			Message:   "redis snapshot seq does not match PostgreSQL auction seq",
			Details: map[string]any{
				"postgres_seq": auction.seq,
				"redis_seq":    valueOrNil(ok, seq),
				"shape":        result.RedisSnapshotShape,
			},
		})
	}

	if eventType != "snapshot" {
		return drifts
	}

	payload, _ := objectField(doc, "payload")
	if payload == nil {
		drifts = append(drifts, fieldDrift(auction.id, "payload", "object", nil))
		return drifts
	}
	if status, ok := stringField(payload, "status"); !ok || status != auction.status {
		drifts = append(drifts, fieldDrift(auction.id, "status", auction.status, valueOrNil(ok, status)))
	}
	if price, ok := int64Field(payload, "current_price_cents"); !ok || price != auction.currentPriceCents {
		drifts = append(drifts, fieldDrift(auction.id, "current_price_cents", auction.currentPriceCents, valueOrNil(ok, price)))
	}
	if winner, ok := nullableStringField(payload, "current_winner_id"); !sameStringPtr(winner, auction.currentWinnerID) || !ok {
		drifts = append(drifts, fieldDrift(auction.id, "current_winner_id", auction.currentWinnerID, valueOrNil(ok, winner)))
	}
	if count, ok := int64Field(payload, "accepted_bid_count"); !ok || count != auction.acceptedBidCount {
		drifts = append(drifts, fieldDrift(auction.id, "accepted_bid_count", auction.acceptedBidCount, valueOrNil(ok, count)))
	}
	if count, ok := int64Field(payload, "extend_count"); !ok || count != auction.extendCount {
		drifts = append(drifts, fieldDrift(auction.id, "extend_count", auction.extendCount, valueOrNil(ok, count)))
	}
	return drifts
}

func validateHistory(auction dbAuction, values []string, result *AuctionResult) []Drift {
	var drifts []Drift
	var previous int64
	for i, value := range values {
		doc, err := decodeObject([]byte(value))
		if err != nil {
			drifts = append(drifts, Drift{
				AuctionID: auction.id,
				Type:      DriftHistoryInvalidJSON,
				Severity:  "HIGH",
				Message:   "redis history entry is not valid JSON",
				Details: map[string]any{
					"index": i,
					"error": err.Error(),
				},
			})
			continue
		}
		seq, ok := int64Field(doc, "seq")
		if !ok {
			drifts = append(drifts, Drift{
				AuctionID: auction.id,
				Type:      DriftHistoryInvalidJSON,
				Severity:  "HIGH",
				Message:   "redis history entry has no seq",
				Details:   map[string]any{"index": i},
			})
			continue
		}
		if i == 0 {
			first := seq
			result.RedisHistoryFirst = &first
		} else if seq != previous+1 {
			drifts = append(drifts, Drift{
				AuctionID: auction.id,
				Type:      DriftHistoryGap,
				Severity:  "HIGH",
				Message:   "redis history has a sequence gap inside the retained window",
				Details: map[string]any{
					"previous_seq": previous,
					"current_seq":  seq,
					"index":        i,
				},
			})
		}
		previous = seq
	}
	if result.RedisHistoryFirst != nil {
		last := previous
		result.RedisHistoryLast = &last
	}
	result.RedisHistoryCount = len(values)
	if previous != auction.seq {
		drifts = append(drifts, Drift{
			AuctionID: auction.id,
			Type:      DriftHistoryLastSeqDrift,
			Severity:  "HIGH",
			Message:   "redis history last seq does not match PostgreSQL auction seq",
			Details: map[string]any{
				"postgres_seq":      auction.seq,
				"redis_history_seq": previous,
			},
		})
	}
	return drifts
}

func (c *Checker) writeAnomalies(ctx context.Context, drifts []Drift) ([]WrittenAnomaly, error) {
	byAuction := map[string][]Drift{}
	for _, drift := range drifts {
		byAuction[drift.AuctionID] = append(byAuction[drift.AuctionID], drift)
	}

	written := make([]WrittenAnomaly, 0, len(byAuction))
	for auctionID, auctionDrifts := range byAuction {
		severity := "MED"
		for _, drift := range auctionDrifts {
			if drift.Severity == "HIGH" || drift.Severity == "CRITICAL" {
				severity = "HIGH"
				break
			}
		}
		payload, err := json.Marshal(map[string]any{
			"drifts": auctionDrifts,
		})
		if err != nil {
			return nil, err
		}
		var id int64
		if err := c.db.QueryRow(ctx, `
			INSERT INTO system_anomaly_events (severity, type, auction_id, message, payload_json)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, severity, AnomalyReconciliationDrift, auctionID, "Redis projection differs from PostgreSQL truth", payload).Scan(&id); err != nil {
			return nil, err
		}
		written = append(written, WrittenAnomaly{
			ID:        id,
			AuctionID: auctionID,
			Type:      AnomalyReconciliationDrift,
		})
	}
	return written, nil
}

func decodeObject(data []byte) (map[string]any, error) {
	var doc map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func ensureRequestedAuctionsFound(requested []string, found []dbAuction) error {
	if len(requested) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(found))
	for _, auction := range found {
		seen[auction.id] = true
	}
	var missing []string
	for _, id := range requested {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return errors.New("requested auction not found: " + fmt.Sprint(missing))
}

func objectField(doc map[string]any, key string) (map[string]any, bool) {
	value, ok := doc[key]
	if !ok {
		return nil, false
	}
	out, ok := value.(map[string]any)
	return out, ok
}

func stringField(doc map[string]any, key string) (string, bool) {
	value, ok := doc[key]
	if !ok || value == nil {
		return "", false
	}
	out, ok := value.(string)
	return out, ok
}

func nullableStringField(doc map[string]any, key string) (*string, bool) {
	value, ok := doc[key]
	if !ok {
		return nil, false
	}
	if value == nil {
		return nil, true
	}
	out, ok := value.(string)
	if !ok {
		return nil, false
	}
	return &out, true
}

func int64Field(doc map[string]any, key string) (int64, bool) {
	value, ok := doc[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int64(typed), typed == float64(int64(typed))
	case int64:
		return typed, true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func fieldDrift(auctionID string, field string, postgres any, redisValue any) Drift {
	return Drift{
		AuctionID: auctionID,
		Type:      DriftSnapshotFieldDrift,
		Severity:  "HIGH",
		Message:   fmt.Sprintf("redis snapshot field %s differs from PostgreSQL", field),
		Details: map[string]any{
			"field":          field,
			"postgres_value": postgres,
			"redis_value":    redisValue,
		},
	}
}

func sameStringPtr(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func valueOrNil[T any](ok bool, value T) any {
	if !ok {
		return nil
	}
	return value
}

func snapshotShape(eventType string, source string) string {
	if eventType == "snapshot" {
		if source == "" {
			return "snapshot"
		}
		return "snapshot:" + source
	}
	if eventType == "" {
		return "unknown"
	}
	return "event_envelope:" + eventType
}

func eventsKey(auctionID string) string {
	return "auction:" + auctionID + ":events"
}

func snapshotKey(auctionID string) string {
	return "auction:" + auctionID + ":snapshot"
}
