# P1-04 Toxiproxy Weak-Network Evidence

Gate: P1-04 Toxiproxy weak-network tests
Date: 2026-05-23 Asia/Shanghai
Base commit: c5e4935

## Design Mapping

- `docs/design-v2-industrial/01-scope-and-roadmap.md`: P1-04 requires Toxiproxy weak-network tests after WS recovery is stable.
- `docs/design-v2-industrial/09-performance-and-benchmark.md`: weak-network and reconnect tests must not turn into unsupported performance claims.
- `docs/design-v2-industrial/10-test-gates.md`: Redis down reconnect, reconnect storm, slow consumer, and DB lock timeout remain failure/degradation gates.
- `docs/design-v2-industrial/12-engineering-rules.md`: PostgreSQL remains auction truth; Redis and WebSocket are projection/delivery only.

## Implemented

Added Docker Compose service:

- `infra/docker-compose.yml` service `toxiproxy`
- Toxiproxy API port `8474`
- PostgreSQL proxy port `15432 -> postgres:5432`
- Redis proxy port `16379 -> redis:6379`

Added chaos assets:

- `tests/chaos/toxiproxy-scenarios.json`
- `tests/chaos/toxiproxy-client.mjs`
- `tests/chaos/run-toxiproxy-scenario.mjs`
- `tests/chaos/validate-toxiproxy-config.mjs`
- `tests/chaos/README.md`

Added script:

- `package.json` script `test:chaos:p1:validate`

Scenarios:

- `redis_latency_reconnect`: Redis downstream latency with jitter.
- `redis_timeout_reconnect`: Redis downstream timeout.
- `postgres_bid_latency`: PostgreSQL downstream latency with jitter.

The runner also supports:

- `--status`: inspect current proxies and toxics.
- `--clear`: recreate base proxies and remove toxics.

## Review Result

`live-auction-v2-code-review` was applied manually against:

- `12-engineering-rules.md`
- `10-test-gates.md`
- `08-observability-and-ops.md`
- `09-performance-and-benchmark.md`
- touched chaos-suite diff

Findings addressed before evidence:

- Added active toxic readback so the runner proves Toxiproxy API state, not only local JSON parsing.
- Added `--clear` so local fault injection does not leak toxics into later tests.
- Removed an explicit `process.exit(0)` path that caused a Windows Node assertion after successful clear output.
- Kept weak-network evidence labeled as local smoke, not production resilience proof.

Current review status: no remaining P0/P1 findings for the P1-04 test-harness slice.

## Verification

Static validation:

```text
pnpm test:chaos:p1:validate
```

Result:

```text
toxiproxy config ok
```

Compose validation:

```text
docker compose -f infra\docker-compose.yml config
```

Result: PASS. Rendered config includes `toxiproxy` with ports `8474`, `15432`, and `16379`.

Real Toxiproxy API smoke:

```text
docker compose -f infra\docker-compose.yml up -d postgres redis toxiproxy
node tests\chaos\run-toxiproxy-scenario.mjs redis_latency_reconnect
node tests\chaos\run-toxiproxy-scenario.mjs --status
node tests\chaos\run-toxiproxy-scenario.mjs postgres_bid_latency
node tests\chaos\run-toxiproxy-scenario.mjs redis_timeout_reconnect
node tests\chaos\run-toxiproxy-scenario.mjs --clear
```

Result: PASS.

Representative active toxic readback:

```json
{
  "scenario": "redis_latency_reconnect",
  "proxy": "redis",
  "toxics": ["redis_downstream_latency"],
  "active_toxics": [
    {
      "name": "redis_downstream_latency",
      "type": "latency",
      "stream": "downstream",
      "toxicity": 1,
      "attributes": {
        "latency": 800,
        "jitter": 200
      }
    }
  ]
}
```

Cleanup readback:

```json
{
  "action": "clear",
  "proxies": {
    "postgres": {
      "listen": "[::]:15432",
      "upstream": "postgres:5432",
      "enabled": true,
      "toxics": []
    },
    "redis": {
      "listen": "[::]:16379",
      "upstream": "redis:6379",
      "enabled": true,
      "toxics": []
    }
  }
}
```

Backend-through-proxy smoke:

```text
DATABASE_URL=postgres://live_auction:live_auction@localhost:15432/live_auction?sslmode=disable
REDIS_ADDR=localhost:16379
HTTP_ADDR=127.0.0.1:18080
go run ./cmd/server
GET http://127.0.0.1:18080/readyz
```

Result: partial PASS for this slice. The readiness response showed:

```json
{
  "postgres": true,
  "redis": true,
  "minio": false,
  "errors": {
    "minio": "context deadline exceeded"
  }
}
```

Interpretation:

- PostgreSQL and Redis connectivity through Toxiproxy ports was proven.
- MinIO readiness timed out in the local environment and is unrelated to the PG/Redis Toxiproxy path.
- This does not prove full H5 reconnect UX under toxics; it proves the reproducible weak-network harness and proxy wiring needed for that gate.

Known limits:

- Scenarios mutate shared Toxiproxy state and should be run sequentially.
- This is not a formal chaos benchmark and contains no QPS/P99/fanout/online-user claim.
- Full reconnect/slow-consumer evidence should run the committed k6/Playwright workloads against a backend configured with `DATABASE_URL` and `REDIS_ADDR` through the proxy ports.

Next action:

- Use this harness in P1 baseline/chaos runs to record raw reconnect and degradation behavior.
- Keep `--clear` in teardown for all future chaos commands.
