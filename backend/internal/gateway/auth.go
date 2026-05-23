package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"live-auction/backend/internal/config"
	apierrors "live-auction/backend/internal/platform/errors"
)

const sessionCookieName = "la_session"

type authUserKey struct{}

type AuthUser struct {
	ID   string
	Role string
}

type AuthHandler struct {
	Config config.Config
	DB     *pgxpool.Pool
}

type loginRequest struct {
	Account string `json:"account"`
	Role    string `json:"role"`
}

func authMiddleware(cfg config.Config, db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hasMockAuthHeader(r) {
				user, ok := mockUserFromRequest(cfg, r)
				if !ok {
					writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "mock auth disabled or invalid", http.StatusUnauthorized))
					return
				}
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authUserKey{}, user)))
				return
			}
			token := sessionTokenFromRequest(r)
			if token == "" {
				writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing session", http.StatusUnauthorized))
				return
			}
			user, err := lookupSession(r.Context(), db, token)
			if err != nil {
				_ = recordAuthSessionExpired(r.Context(), db, r, err)
				writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "invalid or expired session", http.StatusUnauthorized))
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authUserKey{}, user)))
		})
	}
}

func recordAuthSessionExpired(ctx context.Context, db *pgxpool.Pool, r *http.Request, cause error) error {
	if db == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"trace_id":   traceID(r.Context()),
		"remote_ip":  clientIP(r),
		"user_agent": r.UserAgent(),
		"reason":     cause.Error(),
	})
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO system_anomaly_events (severity, type, message, payload_json)
		VALUES ('MED', 'AUTH_SESSION_EXPIRED', 'auth session missing, expired, revoked, or invalid', $1)
	`, payload)
	return err
}

func mockAuthMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := mockUserFromRequest(cfg, r)
			if !ok {
				writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "mock auth disabled", http.StatusUnauthorized))
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authUserKey{}, user)))
		})
	}
}

func (h AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "invalid json body", http.StatusBadRequest))
		return
	}
	userID := demoAccountUserID(h.Config, req.Account, req.Role)
	if userID == "" {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "unknown demo account", http.StatusBadRequest))
		return
	}
	var user AuthUser
	if err := h.DB.QueryRow(r.Context(), `
		SELECT id, role
		FROM users
		WHERE id = $1 AND role IN ('host','user')
	`, userID).Scan(&user.ID, &user.Role); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "demo account not found", http.StatusUnauthorized))
		return
	}
	if req.Role != "" && req.Role != user.Role {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "demo account role mismatch", http.StatusUnauthorized))
		return
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "failed to create session", http.StatusInternalServerError))
		return
	}
	ttl := h.Config.SessionTTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)
	if _, err := h.DB.Exec(r.Context(), `
		INSERT INTO auth_sessions (id, user_id, role, token_hash, expires_at, created_ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "sess_"+uuid.NewString(), user.ID, user.Role, tokenHash, expiresAt, clientIP(r), r.UserAgent()); err != nil {
		writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "failed to persist session", http.StatusInternalServerError))
		return
	}
	setSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":          user,
		"expires_at":    expiresAt,
		"expires_in_ms": int64(time.Until(expiresAt) / time.Millisecond),
	})
}

func (h AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := sessionTokenFromRequest(r)
	if token != "" {
		_, _ = h.DB.Exec(r.Context(), `
			UPDATE auth_sessions
			SET revoked_at = now()
			WHERE token_hash = $1 AND revoked_at IS NULL
		`, hashSessionToken(token))
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, r, apierrors.New(apierrors.CodeUnauthorized, "missing auth user", http.StatusUnauthorized))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func requireHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(authUserKey{}).(AuthUser)
		if !ok || user.Role != "host" {
			writeError(w, r, apierrors.New(apierrors.CodeForbiddenRoom, "host role required", http.StatusForbidden))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) (AuthUser, bool) {
	user, ok := r.Context().Value(authUserKey{}).(AuthUser)
	return user, ok
}

func mockUserFromRequest(cfg config.Config, r *http.Request) (AuthUser, bool) {
	if !mockAuthAllowed(cfg) {
		if r.Header.Get("X-Mock-Role") != "" || r.Header.Get("X-Mock-User-Id") != "" {
			return AuthUser{}, false
		}
		return AuthUser{}, false
	}
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
		return AuthUser{}, false
	}
	return AuthUser{ID: userID, Role: role}, true
}

func mockAuthAllowed(cfg config.Config) bool {
	return cfg.AllowMockAuth || cfg.AppEnv == "test"
}

func hasMockAuthHeader(r *http.Request) bool {
	return r.Header.Get("X-Mock-Role") != "" || r.Header.Get("X-Mock-User-Id") != ""
}

func demoAccountUserID(cfg config.Config, account string, role string) string {
	switch strings.ToLower(strings.TrimSpace(account)) {
	case "host", "demo_host":
		return cfg.MockHostUserID
	case "user", "viewer", "demo_user":
		return cfg.MockUserID
	}
	switch role {
	case "host":
		return cfg.MockHostUserID
	case "user":
		return cfg.MockUserID
	default:
		return ""
	}
}

func sessionTokenFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func lookupSession(ctx context.Context, db *pgxpool.Pool, token string) (AuthUser, error) {
	var user AuthUser
	err := db.QueryRow(ctx, `
		SELECT user_id, role
		FROM auth_sessions
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`, hashSessionToken(token)).Scan(&user.ID, &user.Role)
	return user, err
}

func newSessionToken() (token string, tokenHash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashSessionToken(token), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
