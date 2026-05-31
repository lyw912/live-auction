# L2 Protocol Stacking — Planned

> Status: PLANNED. Not yet implemented. See `tests/pts/MANIFEST.md` for the full framework.

## Purpose

L2 adds background protocol traffic on top of the hot bid path established in L1-C1.
Each sub-test adds one protocol dimension at a time to attribute any new latency regression
to the specific protocol interaction, not to the bid engine itself.

## Workloads

### L2-P1 — Bid + WebSocket fanout

**Hypothesis**: WS goroutine / message fan-out does not back-pressure the bid decision path.

| Dimension | Value |
|---|---|
| Bid VUs | 1000 (same as L1-C1) |
| WS viewer VUs | 500–2000 long-lived connections |
| Auction | 1 hot auction, `auc_live` |
| Duration | 3 min: 1 min ramp-up + 1 min sustained + 1 min ramp-down |
| SLA | bid p99 ≤ 55ms (5ms headroom vs L1-C1 for fanout overhead) |

Pass criteria:
- bid p99 ≤ 55ms
- zero WS message loss (all `auction_state` pushes delivered to connected viewers)
- no goroutine leak (runtime.NumGoroutine stable after ramp-down)

JMX file (to be created): `tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx`

Implementation note: WS viewers connect via `/ws/ticket` → WebSocket. Each viewer subscribes
to auction events. k6 or a JMeter WebSocket sampler runs the viewer thread group in parallel.

---

### L2-P2 — Bid + Read traffic

**Hypothesis**: concurrent read traffic (auction detail, leaderboard, bid history) does not
degrade bid path through DB connection pool or Redis read/write contention.

| Dimension | Value |
|---|---|
| Bid VUs | 1000 |
| Read VUs | 2000–5000 (auction snapshot, leaderboard, bid history polling) |
| Duration | 3 min sustained |
| SLA | bid p99 ≤ 55ms; read p99 ≤ 200ms |

JMX file (to be created): `tests/pts/L2-protocol/pts-2p2-bid-plus-reads.jmx`

---

### L2-P3 — Bid + WS + Reads (combined L2)

**Hypothesis**: L2-P1 and L2-P2 effects do not compound beyond 10ms additional bid p99.

| Dimension | Value |
|---|---|
| Bid VUs | 1000 |
| WS viewer VUs | 1000 |
| Read VUs | 2000 |
| SLA | bid p99 ≤ 60ms |

JMX file (to be created): `tests/pts/L2-protocol/pts-2p3-bid-ws-reads.jmx`

## Prerequisites

- L1-C1 must pass before running any L2 workload.
- WS token pool must include viewer-role sessions (extend `pts-1ab-1000vu-sessions.csv`
  or create a separate viewer session CSV).
- Update `reset-l4b-final-second-pressure.sh` with an `L4B_PROFILE=l2-p1` branch once
  the JMX is authored.
