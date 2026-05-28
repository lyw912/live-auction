package observability

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRegistryRendersPrometheusText(t *testing.T) {
	reg := NewRegistry()
	reg.Inc("auction_bid_request_total", map[string]string{"result": "ACCEPTED", "reason": ""})
	reg.Set("auction_ws_connections", 2, map[string]string{"room": "auc_test"})
	reg.Observe("auction_bid_latency_seconds", 0.02, nil, []float64{0.01, 0.05})

	text := string(reg.Render(context.Background()))
	for _, want := range []string{
		`auction_bid_request_total{reason="",result="ACCEPTED"} 1`,
		`auction_ws_connections{room="auc_test"} 2`,
		`auction_bid_latency_seconds_bucket{le="0.01"} 0`,
		`auction_bid_latency_seconds_bucket{le="0.05"} 1`,
		`auction_bid_latency_seconds_count 1`,
		`runtime_goroutines`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics text missing %q in:\n%s", want, text)
		}
	}
}

func TestSetAdmissionConfigRendersRealtimeHeartbeatLimits(t *testing.T) {
	old := Default
	t.Cleanup(func() { Default = old })
	Default = NewRegistry()

	SetAdmissionConfig(AdmissionConfig{
		Enabled:              true,
		WSQueueMessages:      256,
		WSQueueBytes:         1 << 20,
		WSRecoveryMaxEvents:  300,
		WSSnapshotRebuildMax: 4,
		WSHeartbeatInterval:  20 * time.Second,
		WSHeartbeatTimeout:   5 * time.Second,
	})

	text := string(Default.Render(context.Background()))
	for _, want := range []string{
		`auction_realtime_config_limit{kind="ws_heartbeat_interval_seconds"} 20`,
		`auction_realtime_config_limit{kind="ws_heartbeat_timeout_seconds"} 5`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics text missing %q in:\n%s", want, text)
		}
	}
}
