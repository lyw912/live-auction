package tracing

import (
	"os"
	"testing"
	"time"
)

func TestNormalizeOTLPDurationEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "3s")

	normalizeOTLPDurationEnv("OTEL_EXPORTER_OTLP_TIMEOUT")

	if got := os.Getenv("OTEL_EXPORTER_OTLP_TIMEOUT"); got != "3000" {
		t.Fatalf("OTEL_EXPORTER_OTLP_TIMEOUT=%q, want 3000", got)
	}
	if got := envDuration("OTEL_EXPORTER_OTLP_TIMEOUT", time.Second); got != 3*time.Second {
		t.Fatalf("envDuration()=%s, want 3s", got)
	}
}
