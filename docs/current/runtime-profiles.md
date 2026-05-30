# Runtime Profiles

> Status: current runtime profile guide, 2026-05-31.

This project intentionally has different runtime profiles. Do not infer architecture readiness from one env file.

## Profiles

| Profile | Env source | Engine | Admission | Purpose |
|---|---|---|---|---|
| Local demo | `.env.example` | `postgres_lane` | enabled | conservative manual demo and everyday development |
| PTS-1B hot path | `.env.pts1b.example` or `tests/pts/reset-l4b-final-second-pressure.sh` | `redis_ledger` | disabled | final-second 1000-user contention pressure |
| Historical PG/guard experiments | old docs/scripts | `postgres_lane` or `redis_guard` | varies | historical diagnosis only |

## Local Demo Profile

`.env.example` is intentionally conservative:

- `BID_ENGINE_MODE=postgres_lane`
- `ADMISSION_ENABLED=true`
- `REDIS_ADDR=localhost:6380`
- Kafka variables are present but not the main demo decision path.

This profile is useful for product walkthroughs and debugging. It must not be cited as current PTS-1B performance evidence.

## PTS-1B Profile

Current PTS-1B requires:

```text
BID_ENGINE_MODE=redis_ledger
ADMISSION_ENABLED=false
REDIS_ADDR=localhost:6380
KAFKA_BROKERS=localhost:9092
KAFKA_BID_TOPIC=auction.bid-events
KAFKA_DLQ_TOPIC=auction.dlq
```

Prefer the scripted sequence in `tests/pts/MANIFEST.md` because it also resets PostgreSQL, Redis, Kafka topics, session CSV, preflight gates, and correctness verification.

Manual use of `.env.pts1b.example` is allowed for local debugging only. Formal evidence still needs reset, preflight, PTS report details, server evidence, and verifier output.

## Why Admission Is Disabled For PTS-1B

PTS-1B is not an admission-protection test. It is a hot-engine decision-path test. Admission must be disabled so pressure reaches Redis/Kafka and correctness gates can classify all intended bids.

If admission `429`, `RATE_LIMITED`, or `BID_AUCTION_TOO_HOT` dominates a run, that run is not PTS-1B success evidence.

## Kafka Is Not Optional For PTS-1B

Redis live decisions must be fenced through Kafka and settled to PostgreSQL. If Kafka append/fence is unknown or failed, the system must expose pending/paused/reconciling state according to `docs/current/performance-correctness-contract.md`; it must not present final settled success.

## Do Not Mix Profiles

Invalid examples:

- Running `pts-1b-contention-burst-1000vu-1m.jmx` against `.env.example` and calling the result current PTS-1B.
- Running with `BID_ENGINE_MODE=redis_ledger` but no Kafka topic verification.
- Comparing a PG-lane run and Redis/Kafka run as if only code changed when admission, CSV, data reset, or PTS script also changed.
