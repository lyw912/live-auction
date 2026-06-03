# Independent k6 ECS Runbook

> Status: operating runbook, 2026-06-04.
> Scope: how to run S1-S5 client pressure from a separate ECS and prove the
> load generator was not the bottleneck.

## 1. Why Use A Separate k6 ECS

Use an independent k6 machine when the claim depends on open-model offered load,
long duration, fault behavior, or reconnect behavior. Running k6 on the service
node is useful for development, but it creates a judge question: did the load
generator steal CPU, file descriptors, ephemeral ports, or network capacity from
the service?

The independent ECS must be monitored too. Client p99 is not clean evidence if
k6 CPU is saturated, `dropped_iterations` grows, the NIC drops packets, or the
generator runs out of sockets.

## 2. ECS Choice

Recommended default: `ecs.c9i.xlarge` in the same region/VPC as the service.

| Spec | vCPU / memory | Use | Verdict |
|---|---:|---|---|
| `ecs.u2a-c1m2.xlarge` | 4 vCPU / 8 GiB | Cheap S2/S4/S5 client pressure | Usable for most HTTP/fault/reconnect runs |
| `ecs.c9i.xlarge` | 4 vCPU / 8 GiB | Higher CPU/network headroom, better for burst/WS sanity | Preferred default |

For 5000-10000 WebSocket proof, do not rely on one 4c8G k6 host as the only
pressure source. Use PTS multi-IP or several k6 hosts. A single 4c8G k6 host is
acceptable for S3 1000-2000 WS sanity if its own CPU, socket, and network metrics
stay healthy.

## 3. Scenario Tool Choice

| Scenario | Recommended pressure source | Why |
|---|---|---|
| `S1` final-second contention | PTS for judge chart; independent k6 for sanity/rerun | PTS gives a stronger paid report for the signature 1000-user burst |
| `S2-long-soak` | independent k6 | Open model, 30-60 min, `dropped_iterations`, cheap long run |
| `S2-convergence-drain` | independent k6 or PTS | Service-side drain gates are the core evidence |
| `S2-capacity-stair` | independent k6 first; optional PTS RPS final chart | k6 finds the knee cheaply; PTS can remove local-generator objections |
| `S2-read-interference` | independent k6 or PTS RPS | HTTP RPS mix, not a WebSocket concurrency problem |
| `S3-live-only-fanout` | PTS VU for 5000-10000 WS; k6 for 1000-2000 WS sanity | Online users are concurrency; PTS multi-IP is cleaner at large WS count |
| `S3-mixed-final-burst` | PTS JMeter | Long connections + final bid burst + readers are expensive and easier to present with PTS sampler charts |
| `S4` fault resilience | independent k6 preferred; local acceptable for dev | Fault is on the service side, but independent k6 avoids resource-sharing doubt |
| `S5` reconnect recovery | independent k6 preferred; local acceptable for dev | A separate client better models weak-network recovery |

Do not rerun every scenario just because the k6 ECS exists. Prioritize
`S2-long-soak`, `S4`, `S5`, and `S2-read-interference`. Keep PTS for S1 and
large S3 unless budget prevents it.

## 4. Install On The k6 ECS

Minimum packages:

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg sysstat jq
```

Install k6 using the official package flow for the OS image. Then install
`node_exporter`:

```bash
sudo useradd --no-create-home --shell /usr/sbin/nologin node_exporter || true
curl -L -o /tmp/node_exporter.tar.gz \
  https://github.com/prometheus/node_exporter/releases/download/v1.9.1/node_exporter-1.9.1.linux-amd64.tar.gz
tar -C /tmp -xzf /tmp/node_exporter.tar.gz
sudo cp /tmp/node_exporter-1.9.1.linux-amd64/node_exporter /usr/local/bin/
```

Create `/etc/systemd/system/node_exporter.service`:

```ini
[Unit]
Description=Node Exporter
After=network.target

[Service]
User=node_exporter
ExecStart=/usr/local/bin/node_exporter
Restart=always

[Install]
WantedBy=multi-user.target
```

Enable it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now node_exporter
```

Service-side Prometheus should scrape the private IP:

```yaml
scrape_configs:
  - job_name: k6-ecs
    static_configs:
      - targets: ['K6_ECS_PRIVATE_IP:9100']
```

## 5. Host Sampling Without Prometheus

Prometheus + Grafana is preferred. If it is not wired yet, run a sidecar sampling
loop during each test and save it beside the k6 summary:

```bash
mkdir -p evidence/k6-host

while true; do
  ts=$(date +%s)
  echo "### $ts" >> evidence/k6-host/host-sample.log
  uptime >> evidence/k6-host/host-sample.log
  free -m >> evidence/k6-host/host-sample.log
  ss -s >> evidence/k6-host/host-sample.log
  cat /proc/net/sockstat >> evidence/k6-host/host-sample.log
  pidstat -durh -C k6 1 1 >> evidence/k6-host/k6-pidstat.log
  sar -n DEV,TCP,ETCP 1 1 >> evidence/k6-host/network-sar.log
  sleep 5
done
```

Stop the loop after k6 exits and keep the logs with the run evidence.

## 6. k6 Output Required For Every Run

Run k6 with machine-readable output:

```bash
k6 run \
  --summary-export evidence/k6-summary.json \
  --out json=evidence/k6-samples.jsonl \
  tests/load/s2-steady-soak.js
```

For judge-facing evidence, preserve:

- `k6-summary.json`;
- `k6-samples.jsonl` or a downsampled/counted derivative;
- k6 console summary;
- `host-sample.log`;
- `k6-pidstat.log`;
- `network-sar.log`;
- service-side evidence from `tests/pts/collect-server-evidence.sh`.

## 7. k6 Host Health Gates

Mark a run as load-generator-contaminated if any of these happens:

- k6 CPU stays near 100% with no idle headroom;
- memory pressure or OOM appears;
- `dropped_iterations` grows beyond the scenario's threshold;
- `vus` reaches `vus_max` and still cannot deliver target RPS;
- NIC errors/drops/retransmits spike during the test;
- open files or socket count approaches the OS limit;
- `TIME_WAIT` or ephemeral-port pressure prevents new connections;
- k6 process RSS/fd/thread count grows without bound during a supposedly steady
  run.

Target statement for a clean run:

> "The pressure host retained CPU and network headroom, k6 reported bounded
> `dropped_iterations`, and socket/fd counts stayed below limits. Therefore the
> client p99 and service metrics can be interpreted as system behavior rather
> than a load-generator bottleneck."

## 8. Metrics To Watch

From k6:

- `dropped_iterations`;
- `vus`, `vus_max`;
- `iterations`, `iteration_duration`;
- `http_req_duration`, `http_req_failed`;
- `data_sent`, `data_received`;
- WebSocket session/connect/message metrics for WS runs.

From the k6 ECS OS:

- CPU idle, iowait, steal;
- memory available and k6 RSS;
- network bytes, packets, errors, drops;
- TCP retransmits;
- open files;
- socket count, `ESTABLISHED`, `TIME_WAIT`;
- k6 process CPU, RSS, fd count, and thread count.

From the service side, keep the normal S1-S5 evidence: Redis pending, Kafka lag,
PostgreSQL settlement, outbox backlog, DB pool wait, Go heap/goroutines/fds,
active WS connections, fanout latency, and verifier output.

