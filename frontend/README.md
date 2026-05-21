# Frontend

React + TypeScript + Vite applications.

## Applications

- `pc-console/`: merchant/host console. Use Arco Design for admin-style forms, tables and diagnostics.
- `mobile-h5/`: bidder mobile H5. Use custom auction UI optimized for realtime state, recovery and bidding CTA behavior.

## Rules

- No optimistic bid success.
- CTA is disabled while pending, recovering or disconnected.
- State labels come from server snapshots/events.
- Client countdowns are display-only and never decide close or winner.
- Mobile layouts must be checked for text overlap before demo.
