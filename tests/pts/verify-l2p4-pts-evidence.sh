#!/usr/bin/env bash
set -euo pipefail

INPUT="${1:?usage: bash tests/pts/verify-l2p4-pts-evidence.sh <report-id|sampling-logs.jsonl>}"
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

EXPECTED_WS="${EXPECTED_WS:-2400}"
MIN_BIDS="${MIN_BIDS:-18000}"
BID_P99_MAX_MS="${BID_P99_MAX_MS:-100}"
READ_P99_MAX_MS="${READ_P99_MAX_MS:-200}"
FANOUT_P99_MAX_MS="${FANOUT_P99_MAX_MS:-1000}"
JOIN_SEGMENT_P99_MAX_MS="${JOIN_SEGMENT_P99_MAX_MS:-1000}"
BID_LABEL="${BID_LABEL:-POST L2-P4 steady bid}"
FANOUT_LABEL="${FANOUT_LABEL:-WS L2-P4 fanout observe final seq}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
failures=0

label_file() {
  local label="$1"
  local out="$tmp_dir/$(printf '%s' "$label" | tr -c 'A-Za-z0-9_' '_').jsonl"
  jq -c --arg label "$label" 'select((.samplerLabel // "") == $label)' "$JSONL" > "$out"
  printf '%s' "$out"
}

count_samples() {
  local file="$1"
  wc -l < "$file" | tr -d ' '
}

count_failed() {
  local file="$1"
  jq -s '[.[] | select(((.success // false) != true) or (((.responseCode // "") | tostring | startswith("2")) | not))] | length' "$file"
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

p99_fanout_message_ms() {
  local file="$1"
  jq -r '
    (.responseMessage // "")
    | capture("MAX_LAT_MS_(?<lat>[0-9]+)")?
    | .lat // empty
  ' "$file" |
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

assert_min_count() {
  local label="$1"
  local min_count="$2"
  local file count
  file="$(label_file "$label")"
  count="$(count_samples "$file")"
  if [ "$count" -lt "$min_count" ]; then
    echo "FAIL $label count=$count minimum=$min_count"
    failures=$((failures+1))
  else
    echo "OK   $label count=$count >= $min_count"
  fi
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

assert_fanout_message_p99_lte() {
  local label="$1"
  local max_ms="$2"
  local file p99
  file="$(label_file "$label")"
  p99="$(p99_fanout_message_ms "$file")"
  if [ "$p99" = "NaN" ]; then
    echo "FAIL $label responseMessage fanout p99 missing"
    failures=$((failures+1))
    return
  fi
  if awk -v p="$p99" -v m="$max_ms" 'BEGIN{exit !(p <= m)}'; then
    echo "OK   $label responseMessage fanout_p99_ms=$p99 <= $max_ms"
  else
    echo "FAIL $label responseMessage fanout_p99_ms=$p99 > $max_ms"
    failures=$((failures+1))
  fi
}

echo "L2-P4 PTS evidence verifier"
echo "source: $JSONL"
echo "expected_ws=$EXPECTED_WS min_bids=$MIN_BIDS"

assert_min_count "$BID_LABEL" "$MIN_BIDS"
assert_all_success "$BID_LABEL"
assert_p99_lte "$BID_LABEL" "$BID_P99_MAX_MS"

assert_exact_count "UX L2-P4 join snapshot load" "$EXPECTED_WS"
assert_all_success "UX L2-P4 join snapshot load"
assert_p99_lte "UX L2-P4 join snapshot load" "$JOIN_SEGMENT_P99_MAX_MS"

assert_exact_count "POST L2-P4 WS ticket" "$EXPECTED_WS"
assert_all_success "POST L2-P4 WS ticket"
assert_p99_lte "POST L2-P4 WS ticket" "$JOIN_SEGMENT_P99_MAX_MS"

assert_exact_count "WS L2-P4 upgrade to first message" "$EXPECTED_WS"
assert_all_success "WS L2-P4 upgrade to first message"
assert_p99_lte "WS L2-P4 upgrade to first message" "$JOIN_SEGMENT_P99_MAX_MS"

assert_exact_count "$FANOUT_LABEL" "$EXPECTED_WS"
assert_all_success "$FANOUT_LABEL"
assert_p99_lte "$FANOUT_LABEL" "$FANOUT_P99_MAX_MS"
assert_fanout_message_p99_lte "$FANOUT_LABEL" "$FANOUT_P99_MAX_MS"

fanout_file="$(label_file "$FANOUT_LABEL")"
bad_fanout="$(jq -r 'select((.responseMessage // "") | startswith("WS_L2P4_FANOUT_ALL_SEQ_OK_FINAL_") | not) | .responseMessage' "$fanout_file" | head -n 5)"
if [ -n "$bad_fanout" ]; then
  echo "FAIL WS fanout samples without all-seq proof:"
  printf '%s\n' "$bad_fanout"
  failures=$((failures+1))
else
  echo "OK   WS fanout samples report all-seq proof with final seq"
fi

for label in "GET auction snapshot" "GET auction leaderboard" "GET my bid history"; do
  assert_all_success "$label"
  assert_p99_lte "$label" "$READ_P99_MAX_MS"
done

if [ "$failures" -ne 0 ]; then
  echo "L2-P4 verdict: FAIL ($failures failed checks)"
  exit 1
fi

echo "L2-P4 verdict: PASS"
