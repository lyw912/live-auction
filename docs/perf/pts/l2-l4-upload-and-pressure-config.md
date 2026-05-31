# L2-L4 PTS Upload And Pressure Configuration

> Status: current runbook draft for Alibaba Cloud PTS/JMeter execution.

## Engine Choice

Use **JMeter pressure test** for formal L2-L4 evidence. These workloads need
CSV splitting, Groovy timing barriers, dynamic idempotency keys, and WebSocket
long connections. The JMX files use JSR223 with Java `HttpClient WebSocket`, so
no third-party JMeter plugin JAR is required.

Use **PTS YML / PTS native** only for the auxiliary `L2-P2 read-only` probe.
It is not formal L2 evidence because it does not run in the same clock window as
the bid burst.

## Shared Target

| Field | Value |
|---|---|
| Region | same VPC / same region as ECS |
| Target host | `172.16.179.112` |
| Port | `18080` |
| Protocol | `http` |
| Backend profile | `BID_ENGINE_MODE=redis_ledger`, `ADMISSION_ENABLED=false` |
| CSV mode | enable Split File for every uploaded CSV |

## Data Preparation

```bash
bash tests/pts/prepare-l2-protocol-pressure.sh
bash tests/pts/prepare-l3-l4-pressure.sh
```

Generated/uploaded CSVs:

| File | Purpose |
|---|---|
| `docs/perf/pts/pts-l2-bidder-1000-sessions.csv` | bidder auth pool |
| `docs/perf/pts/pts-l2-viewer-2000-sessions.csv` | WebSocket viewer auth pool |
| `docs/perf/pts/pts-l2-reader-5000-sessions.csv` | HTTP reader auth pool |

## Formal JMeter Upload Matrix

| Workload | Upload files | JMeter properties | Alibaba panel |
|---|---|---|---|
| L2-P1 bid + WS | `tests/pts/L2-protocol/pts-2p1-bid-plus-ws-fanout.jmx`, bidder CSV, viewer CSV | `host=172.16.179.112`, `port=18080`, `bid_threads=1000`, `ws_threads=1000`, `run_duration_sec=180` | JMeter pressure test; duration 3 min; do not use native VU/RPS panel |
| L2-P2 bid + reads | `tests/pts/L2-protocol/pts-2p2-bid-plus-reads.jmx`, bidder CSV, reader CSV | `bid_threads=1000`, `read_threads=2000`, `read_delay_ms=250`, `run_duration_sec=180` | JMeter pressure test; duration 3 min |
| L2-P3 combined | `tests/pts/L2-protocol/pts-2p3-bid-ws-reads.jmx`, all three CSVs | `bid_threads=1000`, `ws_threads=1000`, `read_threads=2000`, `run_duration_sec=180` | JMeter pressure test; duration 3 min |
| L3-S1 lifecycle | `tests/pts/L3-scenario/pts-3s1-full-lifecycle-30min.jmx`, all three CSVs | `bid_threads=500`, `bid_loops=12`, `ws_threads=500`, `read_threads=1000`, `run_duration_sec=1800` | JMeter pressure test; duration 30 min |
| L3-S2 multi-room | `tests/pts/L3-scenario/pts-3s2-multi-room-isolation.jmx`, bidder CSV, reader CSV | `bid_threads=900`, `read_threads=300`, `run_duration_sec=300` | JMeter pressure test; duration 5 min |
| L4-M1 full mixed | `tests/pts/L4-combined/pts-4m1-full-mixed.jmx`, all three CSVs | `bid_threads=1000`, `ws_threads=1000`, `read_threads=3000`, `side_bid_threads=200`, `run_duration_sec=600` | JMeter pressure test; duration 10 min; requires >5000 VU quota |

## If Using The Limited Native PTS Panel

Only use it for `tests/pts/L2-protocol/pts-2p2-read-only.pts.yml`.

Suggested panel values:

| Field | Value |
|---|---|
| Scenario type | PTS YML |
| Pressure mode | RPS mode for read-only probe |
| Traffic model | manual or step increase |
| Max VU | start with 1000 if account quota is limited |
| Duration | 3 minutes |
| Loop count | no fixed loop for sustained reads |

Do not use native PTS for L2-P1/L2-P3/L3/L4 formal evidence: it cannot express
the WebSocket long-connection and bid timing-barrier semantics captured in the
JMeter scripts.

## Post-Run Evidence

```bash
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh <report-id-or-label>
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh <report-id-or-label>
```

For L3-S2/L4, also inspect per-auction rows for `auc_live`, `auc_side`, and
`auc_inv_001`; the existing verifier is centered on the hot auction gate.
