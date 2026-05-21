# Evidence Record

Feature/Gate: P0-02 DB migrations

Date: 2026-05-22

Commit: pending

Environment: Windows local dev, Docker Compose PostgreSQL 16 Alpine

Command:

```powershell
goose -dir migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" up
goose -dir migrations postgres "postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable" status
```

Raw Output Path: this record

## Setup

PostgreSQL was already running from `infra/docker-compose.yml`.

## Expected Invariant

Core P0 tables, constraints and indexes from `docs/design-v2-industrial/04-data-and-storage.md` can be applied by goose.

## Result

PASS

## Observed Data

```text
OK   202605220001_init_core.sql (112.21ms)
goose: successfully migrated database to version: 202605220001

Applied At                  Migration
=======================================
Thu May 21 18:48:06 2026 -- 202605220001_init_core.sql
```

## Failure Interpretation

None.

## Known Limits

This verifies schema application only. It does not yet cover bid transaction behavior, outbox relay, scheduler, WebSocket recovery or frontend flows.

## Next Action

Implement Milestone 1 CRUD/rule lifecycle and Milestone 2 bid truth path tests.
