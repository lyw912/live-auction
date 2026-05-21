package gateway

import (
	"context"
	"net/http"

	"live-auction/backend/internal/config"
	apierrors "live-auction/backend/internal/platform/errors"
)

type authUserKey struct{}

type AuthUser struct {
	ID   string
	Role string
}

func mockAuthMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := r.Header.Get("X-Mock-Role")
			if role == "" {
				role = "host"
			}
			userID := r.Header.Get("X-Mock-User-Id")
			if userID == "" {
				if role == "user" {
					userID = cfg.MockUserID
				} else {
					userID = cfg.MockHostUserID
				}
			}
			if role != "host" && role != "user" {
				writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "invalid mock role", 401))
				return
			}
			ctx := context.WithValue(r.Context(), authUserKey{}, AuthUser{ID: userID, Role: role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requireHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(authUserKey{}).(AuthUser)
		if !ok || user.Role != "host" {
			writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "host role required", 403))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) (AuthUser, bool) {
	user, ok := r.Context().Value(authUserKey{}).(AuthUser)
	return user, ok
}
