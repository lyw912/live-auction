# ADR P2-01 · Session Auth Boundary

Date: 2026-05-23 Asia/Shanghai

Status: Accepted

## Context

P1 still allowed normal runtime identity through `X-Mock-Role` and `X-Mock-User-Id`. That was acceptable for P0/P1 local proof, but it weakens every later ACL, rate-limit, payment, and diagnostics claim because callers can forge identity without a real server-side session.

## Decision

- `users` remains the identity source.
- `auth_sessions` stores only SHA-256 session token hashes, role snapshot, expiry, revoke time, created IP, and user agent.
- `POST /api/auth/login` issues a local demo session for seeded host/user accounts.
- Session is accepted from an HttpOnly `la_session` cookie or `Authorization: Bearer` for non-browser tooling.
- `POST /api/auth/logout` revokes the current session and clears the cookie.
- Normal runtime rejects mock headers unless `ALLOW_MOCK_AUTH=true` or `APP_ENV=test`.
- H5 and PC auto-login with demo accounts, then call backend APIs using the session cookie.

## Consequences

- P2-02 ACL can trust `currentUser` as a server-side session subject.
- Mock auth remains available for package tests and explicit local debugging.
- This is not OAuth/SMS/account binding; it is a real local session boundary for demo and tests.
- Session tokens are not persisted or logged in plaintext.

## Follow-Up Gates

- P2-02 must add room membership and host ownership checks on top of this identity.
- P2-04 rate limits should key on session user plus IP.
- P2-05 payment provider callbacks must not derive payer identity from client-controlled data.
