# L2 Protocol Stacking — Planned

> Status: PLANNED. Not yet implemented. See `tests/pts/MANIFEST.md` for the full framework.

## Purpose

L2 adds background protocol traffic on top of the hot bid path established in L1-C1.
Each sub-test adds one protocol dimension at a time to attribute any new latency regression
to the specific protocol interaction, not to the bid engine itself.

## Workloads

### L2-P1 — Bid + WebSocket fanout

**Hypothesis**: WS goroutine / message fan-out does not back-pressure the bid decision path.

Real business scenario: one high-intent live auction room near the final bidding
moment. Many users watch the price ladder through long-lived WebSocket
connections; a smaller subset actively bids. This is not a generic livestream
viewer count test. It is a protocol-stacking test that asks whether realtime
fanout to watchers slows the synchronous bid decision path.

| Dimension | Value |
|---|---|
| Bid VUs | 1000 (same as L1-C1) |
| WS viewer VUs | 8000–9000 for the first formal run under a 30000 VUM budget; 10000/20000 only as shorter exploratory capacity probes |
| Auction | 1 hot auction, `auc_live` |
| Duration | 3 min: 1 min ramp-up + 1 min sustained + 1 min ramp-down |
| SLA | bid p99 ≤ 100ms hard UX ceiling; server-core bid p99 target ≤ 60ms; WS delivery lag p99 ≤ 2s |

Pass criteria:
- all 1000 bid attempts return final `ENGINE_*` decisions and pass L1 verifier gates
- no `PROCESSING_RETRY_LATER`/`RECONCILING` dominance
- bid p99 ≤ 100ms from PTS sampling logs; server-core gateway p99 reported separately
- zero unexplained WS connection failures during steady state
- WS viewers receive post-bid server-authoritative state, or the gap is explained by sampling/connection churn
- no goroutine leak (runtime.NumGoroutine stable after ramp-down)

JMX file: `tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx`

Implementation note: WS viewers connect via `/api/auth/ws-ticket` and then
`/ws?room_id=room_main&auction_id=auc_live&last_seq=0`. The current JMX uses
Java `HttpClient` WebSocket from JSR223, so no third-party JMeter WebSocket
plugin is required.

---

### L2-P2 — Bid + Read traffic

**Hypothesis**: concurrent HTTP read traffic (auction detail, leaderboard, bid
history) does not degrade bid path through DB connection pool, Redis
read/write contention, or handler CPU.

| Dimension | Value |
|---|---|
| Bid VUs | 1000 |
| Read VUs | 2000–5000 (auction snapshot, leaderboard, bid history polling) |
| Duration | 3 min sustained |
| SLA | bid p99 ≤ 55ms; read p99 ≤ 200ms |

JMX file (to be created): `tests/pts/L2-protocol/pts-2p2-bid-plus-reads.jmx`

---

### L2-P3 — Bid + WS + Reads (combined L2)

**Hypothesis**: L2-P1 and L2-P2 effects do not compound beyond the user-visible
100ms bid p99 hard ceiling.

| Dimension | Value |
|---|---|
| Bid VUs | 1000 |
| WS viewer VUs | 5000–8000, only after L2-P1 passes at that viewer scale |
| Read VUs | 2000 |
| SLA | bid p99 ≤ 100ms; read p99 ≤ 200ms; WS delivery lag p99 ≤ 2s |

JMX file (to be created): `tests/pts/L2-protocol/pts-2p3-bid-ws-reads.jmx`

## Prerequisites

- L1-C1 must pass before running any L2 workload.
- WS token pool must include viewer-role sessions. Use
  `docs/perf/pts/pts-l2-viewer-10000-sessions.csv` from
  `prepare-l2-protocol-pressure.sh`.
- Update `reset-l4b-final-second-pressure.sh` with an `L4B_PROFILE=l2-p1` branch once
  the JMX is authored.
