package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/config"
	"live-auction/backend/internal/outbox"
	"live-auction/backend/internal/realtime"
)

type report struct {
	CommitHint     string         `json:"commit_hint"`
	GeneratedAt    time.Time      `json:"generated_at"`
	Environment    environment    `json:"environment"`
	FinalSecondBid workloadResult `json:"final_second_bid_burst"`
	OutboxBurst    workloadResult `json:"outbox_burst"`
	Fanout         fanoutResult   `json:"watcher_fanout_and_slow_consumer"`
	KnownLimits    []string       `json:"known_limits"`
}

type environment struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	GoVersion  string `json:"go_version"`
	CPUs       int    `json:"cpus"`
	Database   string `json:"database"`
	RedisAddr  string `json:"redis_addr"`
	NativeTool string `json:"native_tool"`
}

type workloadResult struct {
	Result          string   `json:"result"`
	Attempts        int      `json:"attempts,omitempty"`
	Accepted        int      `json:"accepted,omitempty"`
	Rejected        int      `json:"rejected,omitempty"`
	Errors          int      `json:"errors"`
	DurationMS      int64    `json:"duration_ms"`
	P50MS           int64    `json:"p50_ms,omitempty"`
	P95MS           int64    `json:"p95_ms,omitempty"`
	P99MS           int64    `json:"p99_ms,omitempty"`
	SeqContinuous   bool     `json:"seq_continuous,omitempty"`
	OutboxProcessed int      `json:"outbox_processed,omitempty"`
	OutboxPending   int      `json:"outbox_pending,omitempty"`
	Notes           []string `json:"notes,omitempty"`
}

type fanoutResult struct {
	Result          string `json:"result"`
	Watchers        int    `json:"watchers"`
	Messages        int    `json:"messages"`
	HealthyReceived int    `json:"healthy_received"`
	SlowClosed      bool   `json:"slow_closed"`
	DurationMS      int64  `json:"duration_ms"`
	Errors          int    `json:"errors"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cfg := config.Load()
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(ctx).Err(); err != nil {
		fatal(err)
	}

	repo := auction.NewRepository(db)
	auctionRow := createActiveAuction(ctx, db, repo)
	result := report{
		CommitHint:  os.Getenv("GIT_COMMIT"),
		GeneratedAt: time.Now().UTC(),
		Environment: environment{
			GOOS:       runtime.GOOS,
			GOARCH:     runtime.GOARCH,
			GoVersion:  runtime.Version(),
			CPUs:       runtime.NumCPU(),
			Database:   "PostgreSQL via DATABASE_URL",
			RedisAddr:  cfg.RedisAddr,
			NativeTool: "go run ./cmd/p0loadsmoke",
		},
		KnownLimits: []string{
			"local smoke baseline only; not a QPS/P99/fanout capacity claim",
			"k6 is not installed in this environment",
			"single process, small workload, local Docker infra",
		},
	}

	result.FinalSecondBid = runBidBurst(ctx, db, repo, auctionRow.ID)
	result.OutboxBurst = runOutboxBurst(ctx, db, rdb, auctionRow.ID)
	result.Fanout = runFanoutSmoke()

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fatal(err)
	}
	if result.FinalSecondBid.Result != "PASS" || result.OutboxBurst.Result != "PASS" || result.Fanout.Result != "PASS" {
		os.Exit(1)
	}
}

func runBidBurst(ctx context.Context, db *pgxpool.Pool, repo *auction.Repository, auctionID string) workloadResult {
	const attempts = 24
	latencies := make([]time.Duration, attempts)
	var accepted atomic.Int32
	var rejected atomic.Int32
	var errors atomic.Int32
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		i := i
		go func() {
			defer wg.Done()
			userID := fmt.Sprintf("load_user_%02d", i)
			amount := int64(10_100 + i*100)
			bid := auction.BidInput{
				ClientBidID:   "load-bid-" + uuid.NewString(),
				AmountCents:   amount,
				ClientSeenSeq: 0,
			}
			bidStart := time.Now()
			resp, err := repo.PlaceBid(ctx, auctionID, userID, bid.ClientBidID, bid, "p0load")
			latencies[i] = time.Since(bidStart)
			if err != nil {
				errors.Add(1)
				return
			}
			if resp.RejectReason != nil {
				rejected.Add(1)
				return
			}
			accepted.Add(1)
		}()
	}
	wg.Wait()
	continuous := auctionSeqContinuous(ctx, db, auctionID)
	result := workloadResult{
		Result:        "PASS",
		Attempts:      attempts,
		Accepted:      int(accepted.Load()),
		Rejected:      int(rejected.Load()),
		Errors:        int(errors.Load()),
		DurationMS:    time.Since(start).Milliseconds(),
		P50MS:         percentileMS(latencies, 0.50),
		P95MS:         percentileMS(latencies, 0.95),
		P99MS:         percentileMS(latencies, 0.99),
		SeqContinuous: continuous,
		Notes: []string{
			"valid concurrent bids intentionally race on one hot auction row",
			"latency values are smoke measurements, not final baseline claims",
		},
	}
	if result.Errors != 0 || !continuous || result.Accepted == 0 {
		result.Result = "FAIL"
	}
	return result
}

func runOutboxBurst(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, auctionID string) workloadResult {
	start := time.Now()
	relay := outbox.NewRelay(db, rdb, "p0load-worker")
	processed := 0
	errors := 0
	for i := 0; i < 200; i++ {
		ok, err := relay.ProcessOne(ctx)
		if err != nil {
			errors++
			break
		}
		if !ok {
			break
		}
		processed++
	}
	pending := countPendingOutbox(ctx, db, auctionID)
	result := workloadResult{
		Result:          "PASS",
		Errors:          errors,
		DurationMS:      time.Since(start).Milliseconds(),
		OutboxProcessed: processed,
		OutboxPending:   pending,
		Notes: []string{
			"processes pending outbox generated by bid burst and setup events",
			"does not claim sustained hot-table throughput",
		},
	}
	if errors != 0 || pending != 0 || processed == 0 {
		result.Result = "FAIL"
	}
	return result
}

func runFanoutSmoke() fanoutResult {
	const watchers = 12
	const messages = 8
	hub := realtime.NewHub(messages)
	auctionID := "load_fanout"
	start := time.Now()
	var healthyReceived atomic.Int32
	var errors atomic.Int32
	var wg sync.WaitGroup
	ready := make(chan struct{})
	for i := 0; i < watchers; i++ {
		sub := hub.Subscribe(auctionID, nil)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			for j := 0; j < messages; j++ {
				select {
				case _, ok := <-sub.Messages():
					if !ok {
						errors.Add(1)
						return
					}
					healthyReceived.Add(1)
				case <-time.After(2 * time.Second):
					errors.Add(1)
					return
				}
			}
			hub.Unsubscribe(auctionID, sub)
		}()
	}
	close(ready)
	for i := 0; i < messages; i++ {
		hub.Publish(context.Background(), auctionID, []byte(fmt.Sprintf(`{"seq":%d}`, i+1)))
	}
	wg.Wait()

	slowHub := realtime.NewHub(1)
	slowClosed := make(chan struct{})
	slow := slowHub.Subscribe(auctionID, func() { close(slowClosed) })
	slowHub.Publish(context.Background(), auctionID, []byte(`{"seq":1}`))
	slowHub.Publish(context.Background(), auctionID, []byte(`{"seq":2}`))
	closed := false
	select {
	case <-slowClosed:
		closed = true
	default:
	}
	slowHub.Unsubscribe(auctionID, slow)
	result := fanoutResult{
		Result:          "PASS",
		Watchers:        watchers,
		Messages:        messages,
		HealthyReceived: int(healthyReceived.Load()),
		SlowClosed:      closed,
		DurationMS:      time.Since(start).Milliseconds(),
		Errors:          int(errors.Load()),
	}
	if result.Errors != 0 || result.HealthyReceived != watchers*messages || !result.SlowClosed {
		result.Result = "FAIL"
	}
	return result
}

func createActiveAuction(ctx context.Context, db *pgxpool.Pool, repo *auction.Repository) auction.Auction {
	if _, err := db.Exec(ctx, `
		UPDATE outbox_delivery
		SET status = 'PUBLISHED', published_at = COALESCE(published_at, now()), locked_by = NULL, locked_until = NULL
		WHERE status <> 'PUBLISHED'
	`); err != nil {
		fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO users (id, role, display_name)
		SELECT 'load_user_' || lpad(i::text, 2, '0'), 'user', 'Load User ' || i
		FROM generate_series(0, 64) AS s(i)
		ON CONFLICT (id) DO NOTHING
	`); err != nil {
		fatal(err)
	}
	roomID := "room_load_" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO rooms (id, host_id, status) VALUES ($1, 'host_1', 'OPEN')`, roomID); err != nil {
		fatal(err)
	}
	item, err := repo.CreateItem(ctx, auction.CreateItemInput{Title: "P0 Load Smoke Item"})
	if err != nil {
		fatal(err)
	}
	created, err := repo.CreateAuction(ctx, auction.CreateAuctionInput{
		RoomID:          roomID,
		ItemID:          item.ID,
		StartPriceCents: 10_000,
		IncrementCents:  100,
		CapPriceCents:   ptrInt64(1_000_000),
		Rule: auction.Rule{
			DurationSeconds:     120,
			ExtendWindowSeconds: 10,
			ExtendBySeconds:     10,
			MaxExtendCount:      3,
			DepositBPS:          1000,
			DepositFloorCents:   5_000,
			DepositCapCents:     50_000,
		},
	}, "p0load")
	if err != nil {
		fatal(err)
	}
	if _, err := repo.Schedule(ctx, created.ID, nil, "p0load"); err != nil {
		fatal(err)
	}
	started, err := repo.Start(ctx, created.ID, "p0load")
	if err != nil {
		fatal(err)
	}
	return started
}

func auctionSeqContinuous(ctx context.Context, db *pgxpool.Pool, auctionID string) bool {
	var count, distinct, minSeq, maxSeq int64
	err := db.QueryRow(ctx, `
		SELECT count(*), count(DISTINCT seq), COALESCE(min(seq), 0), COALESCE(max(seq), 0)
		FROM auction_events
		WHERE auction_id = $1
	`, auctionID).Scan(&count, &distinct, &minSeq, &maxSeq)
	if err != nil {
		return false
	}
	return count > 0 && count == distinct && maxSeq-minSeq+1 == count
}

func countPendingOutbox(ctx context.Context, db *pgxpool.Pool, auctionID string) int {
	var pending int
	err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_events e
		JOIN outbox_delivery d ON d.outbox_id = e.id
		WHERE e.auction_id = $1 AND d.status <> 'PUBLISHED'
	`, auctionID).Scan(&pending)
	if err != nil {
		return -1
	}
	return pending
}

func percentileMS(values []time.Duration, pct float64) int64 {
	clean := make([]time.Duration, 0, len(values))
	for _, value := range values {
		if value > 0 {
			clean = append(clean, value)
		}
	}
	if len(clean) == 0 {
		return 0
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i] < clean[j] })
	index := int(float64(len(clean)-1) * pct)
	return clean[index].Milliseconds()
}

func ptrInt64(value int64) *int64 {
	return &value
}

func fatal(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "p0loadsmoke: %v\n", err)
	os.Exit(1)
}
