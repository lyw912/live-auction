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

Redis 7 is required for the Redis ledger bidding engine because L4b uses Redis Streams/XADD and consumer groups. Some Windows hosts also run an old Redis service on `localhost:6379`; do not use that service for L4b if it does not support XADD.
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
