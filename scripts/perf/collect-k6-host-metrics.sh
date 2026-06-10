#!/usr/bin/env bash
# Sample the independent k6 host while a workload is running.
set -euo pipefail

OUT_DIR="${1:-${K6_HOST_METRICS_DIR:-artifacts/pts/evidence/incoming/k6-host-$(date +%Y%m%dT%H%M%S)}}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-5}"
K6_PATTERN="${K6_PATTERN:-k6}"

mkdir -p "$OUT_DIR"

HOST_LOG="${OUT_DIR}/host-sample.log"
PIDSTAT_LOG="${OUT_DIR}/k6-pidstat.log"
SAR_DEV_LOG="${OUT_DIR}/network-sar-dev.log"
SAR_TCP_LOG="${OUT_DIR}/network-sar-tcp.log"
SOCKET_LOG="${OUT_DIR}/socket-sample.log"
FD_LOG="${OUT_DIR}/k6-fd-sample.log"

echo "out_dir=${OUT_DIR}"
echo "interval_seconds=${INTERVAL_SECONDS}" > "${OUT_DIR}/collector-env.txt"
echo "k6_pattern=${K6_PATTERN}" >> "${OUT_DIR}/collector-env.txt"
date -Is >> "${OUT_DIR}/collector-env.txt"
uname -a >> "${OUT_DIR}/collector-env.txt"
ulimit -n >> "${OUT_DIR}/collector-env.txt"

sample_fds() {
  local pids pid fd_count thread_count rss_kb
  pids="$(pgrep -f "$K6_PATTERN" || true)"
  if [ -z "$pids" ]; then
    printf '%s\tno_k6_process\n' "$(date +%s)" >> "$FD_LOG"
    return 0
  fi
  for pid in $pids; do
    if [ -d "/proc/${pid}" ]; then
      fd_count="$(find "/proc/${pid}/fd" -maxdepth 1 -type l 2>/dev/null | wc -l | tr -d ' ')"
      thread_count="$(find "/proc/${pid}/task" -maxdepth 1 -mindepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')"
      rss_kb="$(awk '/VmRSS:/ {print $2}' "/proc/${pid}/status" 2>/dev/null || echo 0)"
      printf '%s\tpid=%s\tfd=%s\tthreads=%s\trss_kb=%s\n' "$(date +%s)" "$pid" "$fd_count" "$thread_count" "$rss_kb" >> "$FD_LOG"
    fi
  done
}

echo "Sampling k6 host metrics into ${OUT_DIR}. Press Ctrl-C after the k6 run finishes."
while true; do
  ts="$(date +%s)"
  {
    echo "### ${ts}"
    date -Is
    uptime
    free -m
    vmstat 1 2 | tail -n 1
    cat /proc/net/sockstat
    cat /proc/net/sockstat6 2>/dev/null || true
    ss -s
  } >> "$HOST_LOG" 2>&1

  {
    echo "### ${ts}"
    ss -ant state established '( sport != :22 )' | wc -l
    ss -ant state time-wait | wc -l
  } >> "$SOCKET_LOG" 2>&1

  sample_fds

  if command -v pidstat >/dev/null 2>&1; then
    pidstat -durh -C "$K6_PATTERN" 1 1 >> "$PIDSTAT_LOG" 2>&1 || true
  fi
  if command -v sar >/dev/null 2>&1; then
    sar -n DEV 1 1 >> "$SAR_DEV_LOG" 2>&1 || true
    sar -n TCP,ETCP 1 1 >> "$SAR_TCP_LOG" 2>&1 || true
  fi

  sleep "$INTERVAL_SECONDS"
done
