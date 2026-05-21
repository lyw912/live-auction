package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type traceIDKey struct{}

func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Request-Id")
		if traceID == "" {
			traceID = newTraceID()
		}
		ctx := context.WithValue(r.Context(), traceIDKey{}, traceID)
		w.Header().Set("X-Request-Id", traceID)
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
