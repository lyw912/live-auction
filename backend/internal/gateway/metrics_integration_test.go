package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"live-auction/backend/internal/auction"
	"live-auction/backend/internal/observability"
	"live-auction/backend/internal/storage"
)

func TestMetricsEndpointExportsRuntimeAndDBBackedMetrics(t *testing.T) {
	observability.Default = observability.NewRegistry()
	db := openMetricsDB(t)
	deps := &storage.Dependencies{Postgres: db, Redis: openMonitorRedis(t)}
	repo := auction.NewRepository(db)
	row := createMonitorAuction(t, repo, db)
	insertMonitorAnomaly(t, db, row.ID)

	router := NewRouter(testConfig(), deps, slog.Default())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"runtime_goroutines",
		`auction_anomaly_total{severity="LOW",type="MONITOR_TEST"}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q in:\n%s", want, body)
		}
	}
}

func openMetricsDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return openMonitorDB(t)
}

func TestMetricsMiddlewareCountsHTTPRequests(t *testing.T) {
	observability.Default = observability.NewRegistry()
	handler := requestLogMiddleware(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/test", nil))

	text := string(observability.Default.Render(context.Background()))
	if !strings.Contains(text, `http_request_total{method="POST",path="/api/test",status="202"} 1`) {
		t.Fatalf("http request metric missing in:\n%s", text)
	}
}
