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

## MediaMTX WebRTC Live Loop

The default live-media path is now a real browser WebRTC loop:

```text
PC console getUserMedia() -> WHIP publish -> MediaMTX -> WHEP playback -> H5 bidder
```

MediaMTX exposes the default path at:

```text
http://127.0.0.1:8889/auction-live/whip
http://127.0.0.1:8889/auction-live/whep
```

The backend descriptor defaults to WHEP and keeps MP4 only as an explicit fallback:

```text
LIVE_DEMO_MEDIA_PROTOCOL=whep
LIVE_DEMO_MEDIA_URL=http://127.0.0.1:8889/auction-live/whep
LIVE_DEMO_MIME_TYPE=application/sdp
LIVE_DEMO_IS_LIVE=true
LIVE_DEMO_LATENCY_MS=800
LIVE_MEDIA_FALLBACK_MP4_URL=/demo/jade-live-loop.mp4
```

For a LAN/mobile device, use HTTPS for the PC console page before expecting camera permission. Browsers allow camera capture on `localhost`, but non-localhost origins require a secure context. Production deployment also needs proper TLS, STUN/TURN/ICE policy, and auth around WHIP/WHEP.

When developing through a remote server, prefer a dedicated OpenSSH tunnel from your local machine instead of VS Code's Ports panel for WebRTC media. The WHIP HTTP request goes through `5277`, but the actual WebRTC ICE TCP connection uses `8189`; if the `8189` tunnel drops or is not forwarded, the browser can show a local camera preview while the WHIP publish request eventually times out:

```bash
ssh -N \
  -L 5277:127.0.0.1:5277 \
  -L 5276:127.0.0.1:5276 \
  -L 8189:127.0.0.1:8189 \
  root@SERVER_IP
```

The older LL-HLS demo path is still available as a fallback profile if needed:

```powershell
$env:LIVE_DEMO_MEDIA_PROTOCOL='ll-hls'
$env:LIVE_DEMO_MEDIA_URL='http://127.0.0.1:8888/auction-live/index.m3u8?cookieCheck=1'
$env:LIVE_DEMO_MIME_TYPE='application/vnd.apple.mpegurl'
$env:LIVE_DEMO_IS_LIVE='true'
$env:LIVE_DEMO_LATENCY_MS='3000'
$env:LIVE_MEDIA_FALLBACK_MP4_URL='/demo/jade-live-loop.mp4'
```
