package invariant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultMaxDetails = 20

type Checker struct {
	db *pgxpool.Pool
}

func NewChecker(db *pgxpool.Pool) *Checker {
	return &Checker{db: db}
}

func (c *Checker) Run(ctx context.Context, opts Options) (Report, error) {
	if opts.MaxDetails <= 0 {
		opts.MaxDetails = defaultMaxDetails
	}
	generatedAt := opts.Now
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	report := Report{
		Status:      StatusPass,
		GeneratedAt: generatedAt,
		Scope: Scope{
			AuctionID: opts.AuctionID,
			RoomID:    opts.RoomID,
		},
	}
	scopeSQL, args := scopePredicate(opts)

	oneShotChecks := []func(context.Context, string, []any, int) (CheckResult, error){
		c.checkAuctionSeqMatchesEvents,
		c.checkAuctionEventSeqContiguous,
		c.checkSingleTerminalEvent,
		c.checkTerminalStatusMatchesEvent,
		c.checkAcceptedBidCountMatches,
		c.checkWinnerMatchesLatestAcceptedBid,
		c.checkSoldOrderMatchesAuction,
		c.checkNonSoldOrderLeak,
		c.checkOutboxCoverage,
		c.checkOutboxNoExtraAuctionEvents,
		c.checkOutboxPayloadMatchesAuctionEvent,
		c.checkOutboxDeliveryCoverage,
		c.checkOutboxDeliveryMirror,
		c.checkOutboxHeadOfLine,
		c.checkOutboxPayloadHash,
		c.checkPublishedDeliveryFields,
		c.checkDeadOutboxAnomaly,
		c.checkBidIdempotency,
		c.checkPaymentIdempotency,
		c.checkRedisEngineSettlementContiguous,
		c.checkRedisEngineSeqMatchesSettlement,
		c.checkRedisEngineLedgerHealthy,
		c.checkRedisEngineEventCoverage,
		c.checkRoomIsolation,
		c.checkRoomActiveUniqueness,
		c.checkRoomNarratingUniqueness,
	}
	for _, check := range oneShotChecks {
		result, err := check(ctx, scopeSQL, args, opts.MaxDetails)
		if err != nil {
			return Report{}, err
		}
		report.add(result)
	}
	return report, nil
}

func scopePredicate(opts Options) (string, []any) {
	var clauses []string
	var args []any
	if opts.AuctionID != "" {
		args = append(args, opts.AuctionID)
		clauses = append(clauses, fmt.Sprintf("a.id = $%d", len(args)))
	}
	if opts.RoomID != "" {
		args = append(args, opts.RoomID)
		clauses = append(clauses, fmt.Sprintf("a.room_id = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func limitArg(args []any, maxDetails int) ([]any, string) {
	out := append([]any{}, args...)
	out = append(out, maxDetails)
	return out, fmt.Sprintf("$%d", len(out))
}

func (c *Checker) queryDetails(ctx context.Context, sql string, args ...any) ([]ViolationDetail, error) {
	rows, err := c.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectDetails(rows)
}

func collectDetails(rows pgx.Rows) ([]ViolationDetail, error) {
	fields := rows.FieldDescriptions()
	var details []ViolationDetail
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		detail := make(ViolationDetail, len(values))
		for i, field := range fields {
			detail[string(field.Name)] = normalizeValue(values[i])
		}
		details = append(details, detail)
	}
	return details, rows.Err()
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}

func countDetails(details []ViolationDetail) int {
	if len(details) == 0 {
		return 0
	}
	if total, ok := details[0]["total"].(int64); ok {
		return int(total)
	}
	if total, ok := details[0]["total"].(int32); ok {
		return int(total)
	}
	return len(details)
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func canonicalJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
