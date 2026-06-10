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
- Apache Kafka: `localhost:9092`

Redis 7 is required for the Redis hot bidding state machine. Local Redis is configured with AOF and `appendfsync always` plus `noeviction` for failure testing. Production needs Sentinel or Redis Cluster with replicas; the local single-node service is only a test topology.

Kafka is required for the L4b durable bid ledger. Redis is the hot state machine; Kafka is the immutable event log; PostgreSQL remains settlement/audit truth. The application does not auto-create Kafka topics by default. Local integration tests create their own test topics explicitly; deployment must create `auction.bid-events` and `auction.dlq` explicitly so partition count, replication factor, retention, and ISR policy are visible configuration.

The local Kafka service is single-node and creates topics with RF=1 / min ISR=1. That is functional evidence only. Production durability posture must use:

- at least 3 brokers;
- bid/DLQ topics with replication factor 3;
- `min.insync.replicas=2`;
- producer `acks=all` / `RequireAll`;
- auction-id message keying so one auction is ordered within a partition;
- unclean leader election disabled;
- DLQ, lag, ISR, and replay/reconciliation monitoring.

Kafka's official configuration documents define producer `acks=all` as waiting for the full in-sync replica set, and topic/broker `min.insync.replicas` as the minimum ISR required for a successful write. Redis official persistence docs describe AOF/RDB as Redis recovery mechanisms; they do not replace Kafka as the cross-system decision WAL for this project.
- MinIO API: `localhost:9000`
- MinIO Console: `http://localhost:9001`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (`admin` / `admin`)

Prometheus scrapes the backend at `host.docker.internal:18080/metrics`.
Start the backend on the default `:18080` address before expecting Grafana panels to show live data.
The dashboard is provisioned from `infra/grafana/dashboards/live-auction-overview.json` and only references metric families emitted by the backend `/metrics` endpoint.
Prometheus alert rules are loaded from `infra/prometheus/rules/live-auction-alerts.yml`.
Runbooks for those local alerts are in `docs/design/05-alert-runbooks.md`; no Alertmanager receiver is configured in this local stack.

Data is stored in Docker named volumes. Use `docker compose -f infra\docker-compose.yml down -v` only when you intentionally want to delete local data.
