# Chaos And L1-F Runbook

> Status: current local fault-injection runbook.

## Evidence Standard

The goal is not "the script exits 0". A useful chaos run must prove:

- the fault reached the system;
- user-visible results are explicit (`ENGINE_*`, `RECONCILING`, bounded failure);
- no normal success is returned without a durable fence;
- the system recovers after clearing the fault;
- verifier output and raw evidence are stored under `docs/perf/pts/evidence/incoming/<label>/`.

## Layers

| Layer | Purpose | Scale | Tool |
|---|---|---:|---|
| L1-F | concurrent bid pressure while killing Redis/Kafka/PG/backend | 200 VU default | `tests/pts/run-pts-1c-concurrent-fault.sh` + k6 |
| Toxiproxy chaos | latency/timeout degradation not tied to VU scale | small bounded probes | `tests/chaos/run-toxiproxy-scenario.mjs --run` |

L1-F and Toxiproxy are complementary. L1-F proves concurrent fault behavior.
Toxiproxy proves degraded dependency behavior through proxy latency/timeout
without requiring large VU counts.

## Prerequisites

Install local tools:

```bash
jq --version
k6 version
```

`k6` is required for L1-F only. If local `k6` is absent, the runner uses
`docker run --network host grafana/k6:latest`, so Docker must be able to pull or
already have that image. Toxiproxy chaos probes use Node `fetch`.

## L1-F

Run one fault at a time:

```bash
FAULT_TYPE=redis bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=kafka bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=both bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=redis-flush bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=pg bash tests/pts/run-pts-1c-concurrent-fault.sh
FAULT_TYPE=settlement bash tests/pts/run-pts-1c-concurrent-fault.sh
```

The runner starts the backend with:

```text
ALLOW_MOCK_AUTH=true
BID_ENGINE_MODE=redis_ledger
ADMISSION_ENABLED=false
```

Pass evidence must include:

- `run.log`
- `k6-stdout.txt`
- `k6-results.json`
- `fault-window.json`
- `l1c-gates.tsv`
- `l4b-correctness.txt`
- `l4b-*-gates.tsv`

## Toxiproxy Chaos

Toxiproxy is intentionally outside the default compose file. Start it only for
chaos tests:

```bash
docker compose -f infra/docker-compose.yml -f infra/docker-compose.toxiproxy.yml up -d toxiproxy
```

Start the backend through proxy ports:

```bash
bash tests/chaos/run-chaos-profile.sh
```

Run scenarios:

```bash
node tests/chaos/run-toxiproxy-scenario.mjs redis_latency_reconnect --run
node tests/chaos/run-toxiproxy-scenario.mjs redis_timeout_reconnect --run
node tests/chaos/run-toxiproxy-scenario.mjs postgres_bid_latency --run
```

Clear toxics:

```bash
node tests/chaos/run-toxiproxy-scenario.mjs --clear
```

Pass evidence must include:

- `chaos-report.json`
- `chaos-gates.tsv`
- active toxic config
- before/after ready and snapshot probes
- user-visible bid result samples

## Claims Not Allowed

- Do not claim production durability from local single-broker Kafka.
- Do not claim PTS-1B latency from L1-F; faults intentionally degrade latency.
- Do not claim dependency degradation proof if the backend did not connect
  through `localhost:15432` / `localhost:16379`.
- Do not cite toxiproxy `--status` alone as evidence; it only proves toxic
  configuration, not system behavior.
