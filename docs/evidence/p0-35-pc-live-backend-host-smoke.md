# PC Live Backend Host Smoke

Feature/Gate: PC live backend host workflow smoke

Date: 2026-05-23

Commit: finalized by this record's commit

Environment: Windows 11, Go 1.26.3, Node 24.9.0, pnpm 11.2.2, local PostgreSQL/Redis/MinIO from Docker Compose

Command: `pnpm test:e2e:h5-live`

Raw Output Path: terminal output in development session

## Setup

The live smoke runner starts the Go backend on `127.0.0.1:18080`, H5 Vite on `127.0.0.1:5176`, and PC Vite on `127.0.0.1:5177`. The `pc-console-live` Playwright project opens the real PC console and uses the Vite proxy to call the live backend.

## Expected Invariant

- PC console can load live room auction data and diagnostics from backend APIs.
- Host can create an item and DRAFT auction through the browser.
- Rule save targets the selected auction instead of a fixed fixture ID.
- Host can schedule, start, and cancel the selected auction through live APIs.
- Diagnostics tabs render live backend data, including auction and outbox rows.

## Result

PASS for the implemented P0 host workflow smoke.

## Observed Data

- Browser rendered seeded `P0 Live Smoke Item`.
- Browser created a unique `Live PC Item ...` auction through live backend APIs.
- Rule save returned success for the selected auction.
- Schedule/start moved the auction to `ACTIVE`.
- Diagnostics `Auctions` showed `ACTIVE`; diagnostics `Outbox` showed auction events.
- Cancel flow moved the selected auction to `CANCELLED`.

## Known Limits

- This is a deterministic local smoke, not a full merchant CMS acceptance test.
- File upload is covered at contract/UI level; this smoke uses an image URL to avoid depending on external object storage behavior.
- Mock host auth uses `X-Mock-Role` and `X-Mock-User-Id`.

## Next Action

Keep this smoke green while P1 expands merchant workflow, room selection, and production auth.
