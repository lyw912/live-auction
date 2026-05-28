package observability

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type labelSet map[string]string

type sampleKey struct {
	name   string
	labels string
}

type histogramState struct {
	buckets []float64
	counts  map[string][]uint64
	sums    map[string]float64
	labels  map[string]labelSet
}

type Registry struct {
	mu         sync.RWMutex
	counters   map[sampleKey]float64
	gauges     map[sampleKey]float64
	labels     map[sampleKey]labelSet
	histograms map[string]*histogramState
	db         *pgxpool.Pool
}

type AdmissionConfig struct {
	Enabled               bool
	BidUserLimit          int
	BidIPLimit            int
	BidAuctionLimit       int
	BidAuctionMaxInFlight int
	BidLaneWorkers        int
	BidLaneQueueSize      int
	BidLaneQueueTimeout   time.Duration
	WSTicketMaxInFlight   int
	WSConnectMaxInFlight  int
	WSQueueMessages       int
	WSQueueBytes          int64
	WSRecoveryMaxEvents   int64
	WSSnapshotRebuildMax  int
	WSHeartbeatInterval   time.Duration
	WSHeartbeatTimeout    time.Duration
}

var Default = NewRegistry()

var DefaultLatencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

func NewRegistry() *Registry {
	return &Registry{
		counters:   map[sampleKey]float64{},
		gauges:     map[sampleKey]float64{},
		labels:     map[sampleKey]labelSet{},
		histograms: map[string]*histogramState{},
	}
}

func (r *Registry) WithDatabase(db *pgxpool.Pool) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.db = db
	return r
}

func Inc(name string, labels map[string]string) {
	Default.Inc(name, labels)
}

func Add(name string, value float64, labels map[string]string) {
	Default.Add(name, value, labels)
}

func Set(name string, value float64, labels map[string]string) {
	Default.Set(name, value, labels)
}

func AddGauge(name string, value float64, labels map[string]string) {
	Default.AddGauge(name, value, labels)
}

func Observe(name string, value float64, labels map[string]string, buckets []float64) {
	Default.Observe(name, value, labels, buckets)
}

func Handler(db *pgxpool.Pool) http.Handler {
	return Default.WithDatabase(db)
}

func SetAdmissionConfig(cfg AdmissionConfig) {
	enabled := 0.0
	if cfg.Enabled {
		enabled = 1
	}
	Set("auction_admission_enabled", enabled, nil)
	Set("auction_admission_config_limit", float64(cfg.BidUserLimit), map[string]string{"kind": "bid_user_per_second"})
	Set("auction_admission_config_limit", float64(cfg.BidIPLimit), map[string]string{"kind": "bid_ip_per_second"})
	Set("auction_admission_config_limit", float64(cfg.BidAuctionLimit), map[string]string{"kind": "bid_auction_per_second"})
	Set("auction_admission_config_limit", float64(cfg.BidAuctionMaxInFlight), map[string]string{"kind": "bid_auction_max_in_flight"})
	Set("auction_bid_lane_config", float64(cfg.BidLaneWorkers), map[string]string{"kind": "workers"})
	Set("auction_bid_lane_config", float64(cfg.BidLaneQueueSize), map[string]string{"kind": "queue_size"})
	Set("auction_bid_lane_config", cfg.BidLaneQueueTimeout.Seconds(), map[string]string{"kind": "queue_timeout_seconds"})
	Set("auction_admission_config_limit", float64(cfg.WSTicketMaxInFlight), map[string]string{"kind": "ws_ticket_max_in_flight"})
	Set("auction_admission_config_limit", float64(cfg.WSConnectMaxInFlight), map[string]string{"kind": "ws_connect_max_in_flight"})
	Set("auction_realtime_config_limit", float64(cfg.WSQueueMessages), map[string]string{"kind": "ws_queue_messages"})
	Set("auction_realtime_config_limit", float64(cfg.WSQueueBytes), map[string]string{"kind": "ws_queue_bytes"})
	Set("auction_realtime_config_limit", float64(cfg.WSRecoveryMaxEvents), map[string]string{"kind": "ws_recovery_max_events"})
	Set("auction_realtime_config_limit", float64(cfg.WSSnapshotRebuildMax), map[string]string{"kind": "ws_snapshot_rebuild_max_in_flight"})
	Set("auction_realtime_config_limit", cfg.WSHeartbeatInterval.Seconds(), map[string]string{"kind": "ws_heartbeat_interval_seconds"})
	Set("auction_realtime_config_limit", cfg.WSHeartbeatTimeout.Seconds(), map[string]string{"kind": "ws_heartbeat_timeout_seconds"})
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
	defer cancel()
	_, _ = w.Write(r.Render(ctx))
}

func (r *Registry) Inc(name string, labels map[string]string) {
	r.Add(name, 1, labels)
}

func (r *Registry) Add(name string, value float64, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, copied := makeKey(name, labels)
	r.counters[key] += value
	r.labels[key] = copied
}

func (r *Registry) Set(name string, value float64, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, copied := makeKey(name, labels)
	r.gauges[key] = value
	r.labels[key] = copied
}

func (r *Registry) AddGauge(name string, value float64, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, copied := makeKey(name, labels)
	r.gauges[key] += value
	r.labels[key] = copied
}

func (r *Registry) Observe(name string, value float64, labels map[string]string, buckets []float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(buckets) == 0 {
		buckets = DefaultLatencyBuckets
	}
	state := r.histograms[name]
	if state == nil {
		state = &histogramState{
			buckets: append([]float64(nil), buckets...),
			counts:  map[string][]uint64{},
			sums:    map[string]float64{},
			labels:  map[string]labelSet{},
		}
		sort.Float64s(state.buckets)
		r.histograms[name] = state
	}
	_, copied := makeKey(name, labels)
	encoded := encodeLabels(copied)
	if _, ok := state.counts[encoded]; !ok {
		state.counts[encoded] = make([]uint64, len(state.buckets)+1)
		state.labels[encoded] = copied
	}
	for i, bucket := range state.buckets {
		if value <= bucket {
			state.counts[encoded][i]++
		}
	}
	state.counts[encoded][len(state.buckets)]++
	state.sums[encoded] += value
}

func (r *Registry) Render(ctx context.Context) []byte {
	r.collectRuntime()
	r.collectDatabase(ctx)

	r.mu.RLock()
	defer r.mu.RUnlock()

	var buf bytes.Buffer
	writeSamples(&buf, r.counters)
	writeSamples(&buf, r.gauges)
	for name, state := range r.histograms {
		writeHistogram(&buf, name, state)
	}
	return buf.Bytes()
}

func (r *Registry) collectRuntime() {
	r.Set("runtime_goroutines", float64(runtime.NumGoroutine()), nil)
	if rssBytes, ok := linuxRSSBytes(); ok {
		r.Set("runtime_rss_bytes", float64(rssBytes), nil)
	}
	if openFDs, ok := linuxOpenFDs(); ok {
		r.Set("runtime_open_fds", float64(openFDs), nil)
	}
}

func (r *Registry) collectDatabase(ctx context.Context) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return
	}
	stat := db.Stat()
	r.Set("db_pool_conns", float64(stat.AcquiredConns()), map[string]string{"state": "acquired"})
	r.Set("db_pool_conns", float64(stat.ConstructingConns()), map[string]string{"state": "constructing"})
	r.Set("db_pool_conns", float64(stat.IdleConns()), map[string]string{"state": "idle"})
	r.Set("db_pool_conns", float64(stat.TotalConns()), map[string]string{"state": "total"})
	r.Set("db_pool_max_conns", float64(stat.MaxConns()), nil)
	r.Set("db_pool_acquire_total", float64(stat.AcquireCount()), nil)
	r.Set("db_pool_empty_acquire_total", float64(stat.EmptyAcquireCount()), nil)
	r.Set("db_pool_canceled_acquire_total", float64(stat.CanceledAcquireCount()), nil)
	r.Set("db_pool_acquire_duration_seconds_total", stat.AcquireDuration().Seconds(), nil)
	r.Set("db_pool_empty_acquire_wait_seconds_total", stat.EmptyAcquireWaitTime().Seconds(), nil)
	rows, err := db.Query(ctx, `
		SELECT type, severity, count(*)
		FROM system_anomaly_events
		GROUP BY type, severity
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var anomalyType string
			var severity string
			var count int64
			if scanErr := rows.Scan(&anomalyType, &severity, &count); scanErr != nil {
				return
			}
			r.Set("auction_anomaly_total", float64(count), map[string]string{"type": anomalyType, "severity": severity})
		}
	}
}

func makeKey(name string, labels map[string]string) (sampleKey, labelSet) {
	copied := labelSet{}
	for k, v := range labels {
		copied[k] = v
	}
	return sampleKey{name: name, labels: encodeLabels(copied)}, copied
}

func encodeLabels(labels labelSet) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "\xff")
}

func writeSamples(buf *bytes.Buffer, samples map[sampleKey]float64) {
	keys := make([]sampleKey, 0, len(samples))
	for key := range samples {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name == keys[j].name {
			return keys[i].labels < keys[j].labels
		}
		return keys[i].name < keys[j].name
	})
	for _, key := range keys {
		fmt.Fprintf(buf, "%s%s %s\n", key.name, formatLabels(parseLabels(key.labels)), formatFloat(samples[key]))
	}
}

func writeHistogram(buf *bytes.Buffer, name string, state *histogramState) {
	labelKeys := make([]string, 0, len(state.counts))
	for encoded := range state.counts {
		labelKeys = append(labelKeys, encoded)
	}
	sort.Strings(labelKeys)
	for _, encoded := range labelKeys {
		counts := state.counts[encoded]
		labels := state.labels[encoded]
		for i, bucket := range state.buckets {
			withLE := cloneLabels(labels)
			withLE["le"] = formatFloat(bucket)
			fmt.Fprintf(buf, "%s_bucket%s %d\n", name, formatLabels(withLE), counts[i])
		}
		withInf := cloneLabels(labels)
		withInf["le"] = "+Inf"
		fmt.Fprintf(buf, "%s_bucket%s %d\n", name, formatLabels(withInf), counts[len(counts)-1])
		fmt.Fprintf(buf, "%s_sum%s %s\n", name, formatLabels(labels), formatFloat(state.sums[encoded]))
		fmt.Fprintf(buf, "%s_count%s %d\n", name, formatLabels(labels), counts[len(counts)-1])
	}
}

func parseLabels(encoded string) labelSet {
	labels := labelSet{}
	if encoded == "" {
		return labels
	}
	for _, part := range strings.Split(encoded, "\xff") {
		key, value, ok := strings.Cut(part, "=")
		if ok {
			labels[key] = value
		}
	}
	return labels
}

func formatLabels(labels labelSet) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, strings.ReplaceAll(labels[key], `"`, `\"`)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func cloneLabels(labels labelSet) labelSet {
	out := labelSet{}
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func linuxOpenFDs() (int, bool) {
	if runtime.GOOS != "linux" {
		return 0, false
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, false
	}
	return len(entries), true
}

func linuxRSSBytes() (uint64, bool) {
	if runtime.GOOS != "linux" {
		return 0, false
	}
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(os.Getpagesize()), true
}
