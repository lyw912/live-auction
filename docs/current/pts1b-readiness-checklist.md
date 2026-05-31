# PTS-1B Final Readiness Checklist

> Status: current pre-run checklist, 2026-05-31.

Do not start a paid PTS-1B run until every required item is checked or explicitly
waived in the run review.

## 1. Source And Workspace

- [ ] Current docs read: `docs/current/README.md`, `architecture.md`, `performance-correctness-contract.md`, `runtime-profiles.md`, `evidence-policy.md`.
- [ ] Current PTS manifest read: `tests/pts/MANIFEST.md`.
- [ ] Run-review template ready: `docs/current/pts-run-review-template.md`.
- [ ] Fault scope decided: `docs/current/fault-injection-runbook.md`.
- [ ] Git SHA recorded.
- [ ] Dirty tree recorded, including whether code/JMX/env files are modified.

## 2. Runtime Profile

- [ ] Not using `.env.example` for PTS-1B.
- [ ] `BID_ENGINE_MODE=redis_ledger`.
- [ ] `ADMISSION_ENABLED=false`.
- [ ] `REDIS_ADDR=localhost:6380`.
- [ ] `KAFKA_BROKERS=localhost:9092`.
- [ ] `KAFKA_BID_TOPIC=auction.bid-events`.
- [ ] `KAFKA_DLQ_TOPIC=auction.dlq`.
- [ ] Backend listens on the URL used by PTS, normally `:18080`.

## 3. Reset And Data

```bash
L4B_PROFILE=pts-1b SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh
```

- [ ] Reset command completed successfully.
- [ ] PostgreSQL pressure auctions are clean and active.
- [ ] Redis hot bid keys for prior runs cleared.
- [ ] Kafka bid/DLQ topics recreated and partition count verified.
- [ ] Session CSV generated: `docs/perf/pts/pts-1ab-1000vu-sessions.csv`.
- [ ] CSV row count matches intended unique bidders.

## 4. Preflight

```bash
BASE_URL=http://127.0.0.1:18080 bash tests/pts/preflight-l4b-pts-guards.sh before-<run-label>
```

- [ ] Preflight output exists under `docs/perf/pts/evidence/incoming/before-<run-label>/`.
- [ ] Kafka topic/fence checks pass.
- [ ] Redis AOF/no-eviction/pending checks are acceptable for the scoped run.
- [ ] Settlement `engine_seq` gates pass.
- [ ] Admission metric confirms disabled for downstream pressure.

## 5. PTS Upload

- [ ] JMX: `tests/pts/pts-1b-contention-burst-1000vu-1m.jmx`.
- [ ] CSV: `docs/perf/pts/pts-1ab-1000vu-sessions.csv`.
- [ ] PTS mode: virtual users.
- [ ] Intended users: 1000.
- [ ] Loop count: one bid per user.
- [ ] Latency objective is final user-visible `ENGINE_*` decision latency, not
      HTTP `202` pending acknowledgement RTT.
- [ ] If the sampler can receive `202`, it either retries/polls with the same
      `client_bid_id` until final `ENGINE_*`, or the run is explicitly labeled
      ingress-only and cannot be `CURRENT_PASS`.
- [ ] PTS report/log sampling settings recorded.
- [ ] PTS target host/port matches backend.

## 6. During Run

- [ ] Record report ID.
- [ ] Record run start/end timestamps.
- [ ] Capture system metrics if possible.
- [ ] If injecting faults, record exact injection timestamp and command.
- [ ] Watch for immediate engine pause, DLQ, Redis errors, Kafka lag, PG restart/latency.

## 7. After Run Evidence

```bash
BASE_URL=http://127.0.0.1:18080 bash tests/pts/collect-server-evidence.sh <report-id-or-label>
FINAL_WAIT_SECONDS=0 bash tests/pts/verify-l4b-pts-correctness.sh <report-id-or-label>
```

- [ ] Evidence exists under `docs/perf/pts/evidence/incoming/<report-id-or-label>/`.
- [ ] PTS report details fetched.
- [ ] Sampling logs fetched if sampling was enabled.
- [ ] `ENGINE_*` distribution extracted.
- [ ] HTTP distribution extracted.
- [ ] Settlement status distribution extracted.
- [ ] Kafka lag/DLQ status recorded.
- [ ] Redis pending/paused/reconciling status recorded.
- [ ] PostgreSQL winner/current price/accepted/rejected counts recorded.

## 8. Pass Criteria

- [ ] 1000 intended unique bids classified.
- [ ] User-visible `ENGINE_*` p99 <= 50ms.
- [ ] `202` / `PROCESSING_RETRY_LATER` RTT is not used as the PTS-1B p99.
- [ ] `pending_ratio` and `timeout_ratio` are reported; dominant pending UX
      fails even if HTTP status is 2xx.
- [ ] Final winner is the highest valid amount.
- [ ] Every low reject is strictly below decision-time current required price, or has another explicit business-rule basis.
- [ ] No unresolved pending append, DLQ, engine pause, or settlement gap.
- [ ] No dominant `PROCESSING_RETRY_LATER`, vague `409`, or seconds-long pending UX.
- [ ] Fault gates pass if the run claims fault readiness.

## 9. Review And Archive

- [ ] Write review using `docs/current/pts-run-review-template.md`.
- [ ] Classify the run using `docs/current/evidence-policy.md`.
- [ ] Move evidence out of `incoming/` into `current/` only if `CURRENT_PASS`.
- [ ] Move failed/partial current runs into `archive/current-adjacent/`.
- [ ] Move harness/preflight-only runs into `archive/harness-only/`.
- [ ] Update `docs/perf/pts/evidence/README.md` and any report review links.
