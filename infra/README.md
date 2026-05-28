# Infra

Local development infrastructure.

```powershell
docker compose -f infra\docker-compose.yml up -d
docker compose -f infra\docker-compose.yml ps
docker compose -f infra\docker-compose.yml down
```

Services:

- PostgreSQL: `localhost:5432`
- Redis 7: `localhost:6380`
- Kafka-compatible Redpanda: `localhost:9092` (admin API: `localhost:9644`)

Redis 7 is required for the Redis hot bidding state machine. Local Redis is configured with AOF and `appendfsync always` plus `noeviction` for failure testing. Production needs Sentinel or Redis Cluster with replicas; the local single-node service is only a test topology.

Kafka/Redpanda is required for the L4b durable bid ledger. Redis is the hot state machine; Kafka is the immutable event log; PostgreSQL remains settlement/audit truth. The application does not auto-create Kafka topics by default; local compose uses `redpanda-init` to create `auction.bid-events` and `auction.dlq` with 16 partitions and single-node replicas. Production must create topics explicitly so partition count, replication factor, retention, and ISR policy are visible configuration. The local Redpanda service is single-node and exists for functional/failure gates only. Production must use replicated brokers, `acks=all`, idempotent producers where supported, sufficient ISR, disabled unclean leader election, DLQ monitoring, and replay/reconciliation runbooks.
- MinIO API: `localhost:9000`
- MinIO Console: `http://localhost:9001`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (`admin` / `admin`)

Prometheus scrapes the backend at `host.docker.internal:8080/metrics`.
Start the backend on the default `:8080` address before expecting Grafana panels to show live data.
The dashboard is provisioned from `infra/grafana/dashboards/live-auction-overview.json` and only references metric families emitted by the backend `/metrics` endpoint.
Prometheus alert rules are loaded from `infra/prometheus/rules/live-auction-alerts.yml`.
Runbooks for those local P1 alerts are in `docs/runbooks/alerts.md`; no Alertmanager receiver is configured in this local stack.

Data is stored in Docker named volumes. Use `docker compose -f infra\docker-compose.yml down -v` only when you intentionally want to delete local data.
