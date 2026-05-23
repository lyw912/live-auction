# P2 Progress

Date: 2026-05-23 Asia/Shanghai

Authoritative roadmap: `docs/design-v2-industrial/16-industrial-p2-p3-roadmap.md`

## Milestones

| ID | Deliverable | Status | Required Evidence |
|---|---|---|---|
| P2-01 | Real session boundary | DONE | `backend/migrations/202605230001_auth_sessions.sql`, `/api/auth/login|logout|me`, disabled mock-auth runtime test, H5/PC session auto-login, `docs/evidence/p2-01-real-session-boundary.md` |
| P2-02 | Room membership and host ownership ACL | NOT_STARTED | membership migration, REST/WS forged-room tests, banned/foreign-room tests, ACL diagnostics |
| P2-03 | Remove fixed room path | NOT_STARTED | PC room selector, H5 room route, two-room e2e, multi-room seed/load env |
| P2-04 | Bid admission control and abuse behavior | NOT_STARTED | Redis limiter tests, idempotency-before-limit test, Redis-down fail-open gate, k6 abuse output |
| P2-05 | Payment provider boundary | NOT_STARTED | provider payment tables, signed callback tests, duplicate/late callback tests, reconciliation job |
| P2-06 | Security and abuse diagnostics | NOT_STARTED | real anomaly producers and PC drilldown for auth/ACL/rate/payment failures |
| P2-07 | Linux baseline round 1 | NOT_STARTED | 3-run raw outputs for required workloads, environment record, bottleneck report |

## Notes

- P2 starts from product/security hardening, not multi-instance scaling.
- Mock auth/payment may remain only as explicitly gated test/demo helpers.
- No final capacity claim is allowed until P2-07 and later P5 baselines are complete.
