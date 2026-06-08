# Manual Demo Evidence 2026-06-08

This directory stores real PC/H5 demo evidence captured against local services, not Playwright route mocks.

## Current Services

- Backend: `http://127.0.0.1:18080`
- H5: `http://127.0.0.1:5276`
- PC console: `http://127.0.0.1:5277`

## Latest Atmosphere Evidence

- `evidence/26-h5-leading-pressure-card.png`: PC demo driver submits a real buyer overtake bid. H5 shows `榜一 我`, `领先中`, and the current highest price from backend state.
- `evidence/27-h5-outbid-pressure-card.png`: PC demo driver submits a real competitor bid. H5 receives the outbid event, expands the race board, opens the bid panel, and shows `被超越` with the next valid bid.
- `evidence/28-h5-duel-final-leading.png`: PC demo driver runs a three-bid duel. H5 ends with the buyer leading again and shows real heat/race-board state.

## Verified Commands

```bash
pnpm --filter mobile-h5 build
pnpm --filter pc-console build
cd backend && GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/ai
cd backend && GOCACHE=/tmp/go-build-cache GOMODCACHE=/tmp/go-mod-cache GOPATH=/tmp/go-path go test ./internal/gateway -run 'TestDemoCompetingBid|TestLiveOps|TestHeatSummary'
```

The gateway integration test requires local PostgreSQL/Redis access.
