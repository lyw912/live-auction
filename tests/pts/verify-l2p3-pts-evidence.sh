#!/usr/bin/env bash
set -euo pipefail

INPUT="${1:?usage: bash tests/pts/verify-l2p3-pts-evidence.sh <report-id|sampling-logs.jsonl>}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if [ -f "$INPUT" ]; then
  JSONL="$INPUT"
else
  JSONL="$ROOT_DIR/docs/perf/pts/evidence/incoming/$INPUT/pts-sampling-logs/sampling-logs.jsonl"
fi

if [ ! -f "$JSONL" ]; then
  echo "sampling log not found: $JSONL" >&2
  exit 2
fi

command -v jq >/dev/null 2>&1 || {
  echo "jq is required" >&2
  exit 127
}

EXPECTED_BIDS="${EXPECTED_BIDS:-1008}"
EXPECTED_WS="${EXPECTED_WS:-4998}"
EXPECTED_FINAL_SEQ="${EXPECTED_FINAL_SEQ:-11}"
BID_P99_MAX_MS="${BID_P99_MAX_MS:-100}"
READ_P99_MAX_MS="${READ_P99_MAX_MS:-200}"
FANOUT_P99_MAX_MS="${FANOUT_P99_MAX_MS:-1000}"
JOIN_SEGMENT_P99_MAX_MS="${JOIN_SEGMENT_P99_MAX_MS:-1000}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

failures=0

label_file() {
  local label="$1"
  local out="$tmp_dir/$(printf '%s' "$label" | tr -c 'A-Za-z0-9_' '_').jsonl"
  jq -c --arg label "$label" 'select((.samplerLabel // "") == $label)' "$JSONL" > "$out"
  printf '%s' "$out"
}

p99_ms() {
  local file="$1"
  jq -r '.elapsedTime // empty | select(type == "number" or type == "string") | tostring' "$file" |
    sort -n |
    awk '
      function ceil(x) { return x == int(x) ? x : int(x) + 1 }
      { n++; v[n] = $1 + 0 }
      END {
        if (n == 0) { print "NaN"; exit }
        idx = ceil(0.99 * n)
        if (idx < 1) idx = 1
        if (idx > n) idx = n
        printf "%.3f\n", v[idx]
      }'
}

count_samples() {
  local file="$1"
  wc -l < "$file" | tr -d ' '
}

count_success() {
  local file="$1"
  jq -s '[.[] | select((.success // false) == true and ((.responseCode // "") | tostring | startswith("2")))] | length' "$file"
}

count_failed() {
  local file="$1"
  jq -s '[.[] | select(((.success // false) != true) or (((.responseCode // "") | tostring | startswith("2")) | not))] | length' "$file"
}

assert_exact_count() {
  local label="$1"
  local expected="$2"
  local file count
  file="$(label_file "$label")"
  count="$(count_samples "$file")"
  if [ "$count" -ne "$expected" ]; then
    echo "FAIL $label count=$count expected=$expected"
    failures=$((failures+1))
  else
    echo "OK   $label count=$count"
  fi
}

assert_all_success() {
  local label="$1"
  local file count failed
  file="$(label_file "$label")"
  count="$(count_samples "$file")"
  if [ "$count" -eq 0 ]; then
    echo "FAIL $label has no samples"
    failures=$((failures+1))
    return
  fi
  failed="$(count_failed "$file")"
  if [ "$failed" -ne 0 ]; then
    echo "FAIL $label failed_samples=$failed"
    jq -r 'select(((.success // false) != true) or (((.responseCode // "") | tostring | startswith("2")) | not)) | [.responseCode, .responseMessage, .threadName] | @tsv' "$file" | head -n 20
    failures=$((failures+1))
  else
    echo "OK   $label all samples succeeded"
  fi
}

assert_p99_lte() {
  local label="$1"
  local max_ms="$2"
  local file p99
  file="$(label_file "$label")"
  p99="$(p99_ms "$file")"
  if [ "$p99" = "NaN" ]; then
    echo "FAIL $label p99 missing"
    failures=$((failures+1))
    return
  fi
  if awk -v p="$p99" -v m="$max_ms" 'BEGIN{exit !(p <= m)}'; then
    echo "OK   $label p99_ms=$p99 <= $max_ms"
  else
    echo "FAIL $label p99_ms=$p99 > $max_ms"
    failures=$((failures+1))
  fi
}

echo "L2-P3 PTS evidence verifier"
echo "source: $JSONL"
echo "expected: bids=$EXPECTED_BIDS ws=$EXPECTED_WS final_seq=$EXPECTED_FINAL_SEQ"

assert_exact_count "POST L2-P3 hot bid" "$EXPECTED_BIDS"
assert_all_success "POST L2-P3 hot bid"
assert_p99_lte "POST L2-P3 hot bid" "$BID_P99_MAX_MS"

assert_exact_count "UX join snapshot load" "$EXPECTED_WS"
assert_all_success "UX join snapshot load"
assert_p99_lte "UX join snapshot load" "$JOIN_SEGMENT_P99_MAX_MS"

assert_exact_count "POST WS ticket" "$EXPECTED_WS"
assert_all_success "POST WS ticket"
assert_p99_lte "POST WS ticket" "$JOIN_SEGMENT_P99_MAX_MS"

assert_exact_count "WS upgrade to first message" "$EXPECTED_WS"
assert_all_success "WS upgrade to first message"
assert_p99_lte "WS upgrade to first message" "$JOIN_SEGMENT_P99_MAX_MS"

assert_exact_count "WS fanout receive all seq" "$EXPECTED_WS"
assert_all_success "WS fanout receive all seq"
assert_p99_lte "WS fanout receive all seq" "$FANOUT_P99_MAX_MS"

fanout_file="$(label_file "WS fanout receive all seq")"
fanout_count="$(count_samples "$fanout_file")"
bad_final_seq="$(jq -r --arg seq "FINAL_${EXPECTED_FINAL_SEQ}" 'select((.responseMessage // "") | contains($seq) | not) | .responseMessage' "$fanout_file" | head -n 5)"
if [ "$fanout_count" -eq 0 ]; then
  echo "FAIL WS fanout receive all seq has no samples to prove final seq coverage"
  failures=$((failures+1))
elif [ -n "$bad_final_seq" ]; then
  echo "FAIL WS fanout receive all seq has samples that did not report FINAL_${EXPECTED_FINAL_SEQ}"
  printf '%s\n' "$bad_final_seq"
  failures=$((failures+1))
else
  echo "OK   WS fanout receive all seq proves each sampled connection received seq 1..$EXPECTED_FINAL_SEQ with latency timestamps"
fi

for label in "GET auction snapshot" "GET auction leaderboard" "GET my bid history"; do
  assert_all_success "$label"
  assert_p99_lte "$label" "$READ_P99_MAX_MS"
done

if [ "$failures" -ne 0 ]; then
  echo "L2-P3 verdict: FAIL ($failures failed checks)"
  exit 1
fi

echo "L2-P3 verdict: PASS"
