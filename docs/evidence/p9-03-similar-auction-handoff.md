# P9-S3 Similar Auction Handoff Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S3 Similar Auction Handoff<br>
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/19-extreme-bidding-atmosphere.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

Expanded H5 loser/unsold result sheets with a room-list handoff card.

The handoff uses the already-loaded room auction list and selects the next visible `SCHEDULED` or `DRAFT` auction that is not the active terminal auction. It renders:

- next auction title;
- next auction status;
- current/start price from the room auction row;
- explicit source label: `Room list handoff`;
- disclaimer that it is not a recommendation algorithm, inventory reservation, or winner priority.

The primary bid CTA remains disabled in loser and unsold terminal states.

## Validation

```text
pnpm --filter mobile-h5 exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --grep "disables bid CTA|server terminal|loser and unsold result" --reporter=line --workers=1
```

Result: PASS, 4 passed. Coverage proves loser and unsold sheets show the next room-list auction, include no-reservation/no-recommendation copy, keep the primary bid CTA disabled, and continue to accept only continuous server terminal event sequences.

```text
pnpm exec playwright test tests/e2e/visual-regression.spec.ts --project=visual-mobile-h5 --update-snapshots --reporter=line --workers=1
```

Result: PASS, 7 passed. H5 sold-loser visual baseline was regenerated for the handoff card.

```text
pnpm build
pnpm test:e2e
```

Result: PASS. `pnpm test:e2e` ran 55 Playwright tests across mobile H5, PC console, and visual projects.

## Review

- No recommendation algorithm is claimed.
- No reserved inventory or priority claim is shown.
- Handoff is driven by the current room auction list already returned by backend APIs.
- Terminal loser/unsold states still disable bidding.
- No backend mutation, bid truth, outbox, WebSocket, order, or payment path changed.

## Known Limits

- The handoff is not a semantic similarity engine. It is a deterministic room-list continuation.
- It does not switch the active auction automatically; full navigation to a different active auction remains governed by room auction loading and server state.
