package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	apptracing "live-auction/backend/internal/tracing"
)

type traceIDKey struct{}

func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := apptracing.ExtractHTTP(r.Context(), r.Header)
		ctx, span := apptracing.StartServer(ctx, "http.request",
			attribute.String("http.request.method", r.Method),
			attribute.String("url.path", r.URL.Path),
		)
		defer span.End()
		traceID := r.Header.Get("X-Request-Id")
		if traceID == "" {
			traceID = apptracing.SpanTraceID(ctx)
			if traceID == "" {
				traceID = newTraceID()
			}
		}
		ctx = context.WithValue(ctx, traceIDKey{}, traceID)
		w.Header().Set("X-Request-Id", traceID)
		if otelTraceID := apptracing.SpanTraceID(ctx); otelTraceID != "" {
			w.Header().Set("X-Trace-Id", otelTraceID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func traceID(ctx context.Context) string {
	value, _ := ctx.Value(traceIDKey{}).(string)
	return value
}

func newTraceID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "tr_unknown"
	}
	return "tr_" + hex.EncodeToString(buf[:])
}
