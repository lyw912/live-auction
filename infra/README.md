# Infra

Local development infrastructure.

```powershell
docker compose -f infra\docker-compose.yml up -d
docker compose -f infra\docker-compose.yml ps
docker compose -f infra\docker-compose.yml down
```

Services:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO API: `localhost:9000`
- MinIO Console: `http://localhost:9001`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (`admin` / `admin`)

Prometheus scrapes the backend at `host.docker.internal:8080/metrics`.
Start the backend on the default `:8080` address before expecting Grafana panels to show live data.
The dashboard is provisioned from `infra/grafana/dashboards/live-auction-overview.json` and only references metric families emitted by the backend `/metrics` endpoint.

Data is stored in Docker named volumes. Use `docker compose -f infra\docker-compose.yml down -v` only when you intentionally want to delete local data.
