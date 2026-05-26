# P9-S2 Verified Bidder UX States Evidence

> Date: 2026-05-27 Asia/Shanghai<br>
> Slice: P9-S2 Verified Bidder UX Hooks<br>
> Reference: `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`, `docs/design-v2-industrial/19-extreme-bidding-atmosphere.md`, `docs/design-v2-industrial/20-ui-ux-redesign.md`

## Changed

Added bidder verification/deposit requirement hooks without claiming backend enforcement that does not exist yet.

H5 now recognizes optional requirement fields on room auction list and snapshot payloads:

```json
{
  "bidder_requirement": {
    "verification_required": true,
    "deposit_required": true,
    "verified": false,
    "deposit_held": false,
    "reason": "高价值拍品需完成买家验证和保证金冻结"
  }
}
```

When the active auction requires verification or deposit and the current user does not satisfy it, H5:

- keeps the price/countdown/rank surface visible;
- disables the primary bid CTA;
- shows clear requirement copy in the bid hint;
- does not submit `POST /api/auctions/{id}/bids`.

PC Seller Studio now shows a disabled `Verified bidder gate` placeholder in the Trust/Deposit section. It explicitly says the backend has no rule field yet and does not write any new auction-rule payload.

## Validation

```text
pnpm --filter mobile-h5 exec tsc --noEmit
pnpm --filter pc-console exec tsc --noEmit
```

Result: PASS.

```text
pnpm exec playwright test tests/e2e/mobile-h5.spec.ts --project=mobile-h5 --grep "verified bidder requirement|disables bid CTA" --reporter=line --workers=1
```

Result: PASS, 2 passed. Added coverage that requirement state disables bid CTA, shows requirement copy, and does not send a bid request.

```text
pnpm exec playwright test tests/e2e/pc-console.spec.ts --project=pc-console --grep "rule save targets" --reporter=line --workers=1
```

Result: PASS, 1 passed. Confirms the PC verified bidder control is a disabled placeholder while existing rule save payload remains unchanged.

## Review

- No backend bid enforcement is claimed or implied.
- No new rule field is sent from PC to backend.
- H5 only blocks the CTA when the server-supplied optional requirement says the current user is not eligible.
- Existing server-authoritative bid path remains unchanged; PostgreSQL is still auction truth.

## Known Limits

- There is no account verification provider, deposit pre-authorization flow, or backend rule enforcement in this slice.
- Future implementation must add backend rule storage/enforcement and audit before this can be marketed as a complete verified-bidder gate.
