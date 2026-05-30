# P8 Progress

> 2026-05-31 supersession notice: this is a historical PC host-console progress ledger. Old PostgreSQL-authority wording is superseded for the current hot-bid path by `docs/current/architecture.md`; product/UI notes remain useful when they do not conflict with current docs.

> Date: 2026-05-27 Asia/Shanghai  
> Scope: Host Live Assist And Seller Studio from `docs/design-v2-industrial/21-p5-plus-atmosphere-ui-execution-roadmap.md`.

## Slice Status

| Slice | Status | Evidence |
|---|---|---|
| P8-S1 PC Command Center Layout | DONE | `docs/evidence/p8-01-pc-command-center-layout.md` |
| P8-S2 Auction Queue And Active Pinning | DONE | `docs/evidence/p8-02-pc-auction-queue-active-pinning.md` |
| P8-S3 Host Prompter Backend | DONE | `docs/evidence/p8-03-host-prompter-backend.md` |
| P8-S4 Host Live Assist UI | DONE | `docs/evidence/p8-04-pc-host-live-assist-panel.md` |
| P8-S5 Seller Rule Wizard And Preview | DONE | `docs/evidence/p8-05-seller-rule-wizard-preview.md` |
| P8-S6 Heat Summary Aggregation | DONE | `docs/evidence/p8-06-live-auction-heat-summary.md` |

## Current Rules

- P8 PC UI does not change auction money truth; PostgreSQL remains authoritative.
- PC command surfaces must keep existing host workflows: item create/upload, auction create, rules save, schedule/start/cancel, narrate, orders, and diagnostics.
- Route-mocked visual tests are UI contract coverage only. Live/backend evidence remains separate.
- S1 uses existing backend APIs and flight-recorder data; host prompter and heat summary are still owned by S3/S6.
- S2 only mirrors ACTIVE/narrating constraints in PC UI. Backend state transition and narrating race checks remain authoritative.
- S3 host prompter is read-only and advisory. It must never mutate auction truth, write outbox, auto-send chat, or enter bid/cancel/payment paths.
- S4 consumes host prompter prompts in PC UI. Local dismiss is not persisted and must not be described as backend audit.
- S5 keeps rule create/update payloads unchanged. The PC wizard is only a safer operator layout and H5 preview; backend DRAFT-only validation remains final authority.
- S6 heat summary is DB-backed and host-only. Watcher count remains explicitly unavailable until a measured producer exists; route-mocked UI tests are contract coverage, not demo evidence.
