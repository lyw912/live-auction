# P2-07 Linux Baseline Round 1

Date: 2026-05-23T18:54:58.201Z

Commit: c5aee15adff08fda3da3d510e9dc66b4c549a7ef

Status: SMOKE_ONLY_NOT_A_CAPACITY_BASELINE

## Environment

```json
{
  "platform": "win32",
  "arch": "x64",
  "cpu": "12th Gen Intel(R) Core(TM) i9-12900H",
  "cpu_count": 20,
  "ram_bytes": 16890978304,
  "os_release": "10.0.26200",
  "kernel": "Windows_NT lyw 10.0 26200 x86_64 MS/Windows",
  "go": "go version go1.26.3 windows/amd64",
  "k6": "k6.exe v2.0.0 (commit/8c3be52cc1, go1.26.3, windows/amd64)",
  "postgres": "'psql' is not recognized as an internal or external command,\r\noperable program or batch file.",
  "redis": "Redis server v=3.2.100 sha=00000000:0 malloc=jemalloc-3.6.0 bits=64 build=dd26f1f93c5130ee",
  "ulimit_n": "not-linux",
  "somaxconn": "not-linux",
  "ephemeral_port_range": "not-linux",
  "git_sha": "c5aee15adff08fda3da3d510e9dc66b4c549a7ef",
  "mode": "local-smoke",
  "duration": "5s",
  "vus": "2"
}
```

## Workloads

| Workload | Run | Status | Raw Output |
|---|---:|---|---|
| final-second-bid-burst | 1 | FAIL | E:/bytedance/live-auction/docs/perf/raw/p2-07/final-second-bid-burst-run-1.json |

## Verdict

Measured claim allowed: no

Known limits:

- Smoke mode exists only to validate the harness and scripts.
- Windows/WSL/local smoke output must not be used for QPS, p99, fanout, or online-user claims.
- Final mode requires Linux native, documented DB/Redis/backend/k6 boundaries, and 3 raw runs per workload.

Next action:

- Run `node tests/load/run-p2-linux-baseline.mjs --final` on the Linux baseline host after starting PostgreSQL, Redis, backend, and seeding with `go run ./cmd/p1loadseed`.
