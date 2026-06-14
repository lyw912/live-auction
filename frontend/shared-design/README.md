# Auction Terminal Shared Design

This package is the Phase 1 shared design surface for both frontends.

## Rules

- Use shadcn-compatible semantic tokens from `tokens.css`: `--background`, `--foreground`, `--card`, `--primary`, `--border`, `--ring`, and chart/sidebar tokens.
- Use auction semantic tokens for business state: `--state-leading`, `--state-outbid`, `--state-won`, `--state-lost`, `--bid-cta`, `--flash-up`, `--flash-down`, `--live`, `--stale`, `--paused`.
- Keep body text off pure white; use `--foreground` and `--muted-foreground`.
- Keep gold limited to winning, prestige, or authoritative sold moments.
- State must not be conveyed by color alone.
- Do not edit primitive `src/components/ui/*` for business variants; wrap them in feature widgets instead.
- Motion OSS APIs are allowed; Motion+ paid components/resources are not.

## Typography Roles

- Display: `var(--font-auction-display)` for lot titles and sold moments.
- UI: `var(--font-auction-sans)` for controls and body copy.
- Tabular: `var(--font-auction-tabular)` for prices, timers, logs, and metrics.

## Compatibility

Legacy variables such as `--color-bid-red`, `--bg-page`, and `--text-1` remain mapped while H5 and PC migrate component by component.
