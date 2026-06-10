#!/usr/bin/env bash
set -euo pipefail

INPUT="${1:?usage: bash tests/pts/verify-s3-pts-evidence.sh <report-id|sampling-logs.jsonl>}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

REPORT_ID=""
if [ -f "$INPUT" ]; then
  JSONL="$INPUT"
  EVIDENCE_DIR="$(dirname "$(dirname "$JSONL")")"
else
  REPORT_ID="$INPUT"
  EVIDENCE_DIR="$ROOT_DIR/artifacts/pts/evidence/incoming/$REPORT_ID"
  JSONL="$EVIDENCE_DIR/pts-sampling-logs/sampling-logs.jsonl"
fi

if [ ! -f "$JSONL" ]; then
  echo "sampling log not found: $JSONL" >&2
  exit 2
fi

command -v jq >/dev/null 2>&1 || {
  echo "jq is required" >&2
  exit 127
}

PTS_REGION="${PTS_REGION:-cn-heyuan}"
PTS_ENDPOINT="${PTS_ENDPOINT:-pts.aliyuncs.com}"
DETAILS="${DETAILS_FILE:-$EVIDENCE_DIR/jmeter-report-details.json}"

details_valid=0
if [ -s "$DETAILS" ] && jq -e 'has("SamplerMetricsList")' "$DETAILS" >/dev/null 2>&1; then
  details_valid=1
fi

if [ -n "$REPORT_ID" ] && [ "$details_valid" -ne 1 ]; then
  command -v aliyun >/dev/null 2>&1 || {
    echo "aliyun CLI is required to fetch report details for report id input" >&2
    exit 127
  }
  mkdir -p "$EVIDENCE_DIR"
  aliyun pts get-jmeter-report-details \
    --endpoint "$PTS_ENDPOINT" \
    --region "$PTS_REGION" \
    --report-id "$REPORT_ID" > "$DETAILS"
fi

EXPECTED_BIDS="${EXPECTED_BIDS:-1008}"
EXPECTED_WS="${EXPECTED_WS:-4998}"
EXPECTED_READERS="${EXPECTED_READERS:-994}"
BID_P99_MAX_MS="${BID_P99_MAX_MS:-100}"
JOIN_SEGMENT_P99_MAX_MS="${JOIN_SEGMENT_P99_MAX_MS:-1000}"
READ_P99_MAX_MS="${READ_P99_MAX_MS:-200}"
FANOUT_MAX_LAT_MS="${FANOUT_MAX_LAT_MS:-1000}"
REQUIRE_EXACT_COUNTS="${REQUIRE_EXACT_COUNTS:-1}"

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
  wc -l < "$1" | tr -d ' '
}

count_failed() {
  jq -s '[.[] | select(((.success // false) != true) or (((.responseCode // "") | tostring | startswith("2")) | not))] | length' "$1"
}

has_report_metric() {
  local label="$1"
  [ -f "$DETAILS" ] && jq -e --arg label "$label" '(.SamplerMetricsList // [])[] | select(.ApiName == $label)' "$DETAILS" >/dev/null
}

report_metric_field() {
  local label="$1"
  local field="$2"
  jq -r --arg label "$label" --arg field "$field" '
    first((.SamplerMetricsList // [])[] | select(.ApiName == $label) | .[$field]) // empty
  ' "$DETAILS"
}

assert_count() {
  local label="$1"
  local expected="$2"
  local severity="${3:-FAIL}"
  local file count source
  if has_report_metric "$label"; then
    count="$(report_metric_field "$label" AllCount)"
    source="report-details"
  else
    file="$(label_file "$label")"
    count="$(count_samples "$file")"
    source="sampling-log"
  fi
  if [ "$REQUIRE_EXACT_COUNTS" = "1" ] && [ "$count" -ne "$expected" ]; then
    echo "FAIL $label count=$count expected=$expected source=$source"
    failures=$((failures+1))
  elif [ "$count" -eq 0 ]; then
    if [ "$severity" = "WARN" ]; then
      echo "WARN $label has no samples source=$source"
    else
      echo "FAIL $label has no samples source=$source"
      failures=$((failures+1))
    fi
  else
    echo "OK   $label count=$count expected=$expected exact_required=$REQUIRE_EXACT_COUNTS source=$source"
  fi
}

assert_all_success() {
  local label="$1"
  local file failed
  if has_report_metric "$label"; then
    failed="$(report_metric_field "$label" FailCountReq)"
    local success_rate
    success_rate="$(report_metric_field "$label" SuccessRateReq)"
    if [ "$failed" -ne 0 ] || ! awk -v s="$success_rate" 'BEGIN{exit !(s == 100)}'; then
      echo "FAIL $label failed_requests=$failed success_rate=$success_rate source=report-details"
      failures=$((failures+1))
    else
      echo "OK   $label all requests succeeded source=report-details"
    fi
    return
  fi
  file="$(label_file "$label")"
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
  local severity="${3:-FAIL}"
  local file p99 source
  if has_report_metric "$label"; then
    p99="$(report_metric_field "$label" Seg99Rt)"
    source="report-details"
  else
    file="$(label_file "$label")"
    p99="$(p99_ms "$file")"
    source="sampling-log"
  fi
  if [ "$p99" = "NaN" ]; then
    if [ "$severity" = "WARN" ]; then
      echo "WARN $label p99 missing source=$source"
    else
      echo "FAIL $label p99 missing source=$source"
      failures=$((failures+1))
    fi
    return
  fi
  if awk -v p="$p99" -v m="$max_ms" 'BEGIN{exit !(p <= m)}'; then
    echo "OK   $label p99_ms=$p99 <= $max_ms source=$source"
  else
    echo "FAIL $label p99_ms=$p99 > $max_ms source=$source"
    failures=$((failures+1))
  fi
}

assert_fanout_response() {
  local label="S3 live fanout receive"
  local file bad max_lat
  file="$(label_file "$label")"
  bad="$(jq -r 'select(((.responseMessage // "") | test("S3(_V[0-9]+)?_LIVE_FANOUT_OK_LIVE_MESSAGES_[1-9][0-9]*_LIVE_SEQS_[1-9][0-9]*_FINAL_SEQ_[0-9]+_MAX_LAT_MS_[0-9]+")) | not) | .responseMessage' "$file" | head -n 20)"
  if [ -n "$bad" ]; then
    echo "FAIL $label has samples without S3 live fanout proof"
    printf '%s\n' "$bad"
    failures=$((failures+1))
    return
  fi
  max_lat="$(jq -r '(.responseMessage // "") | capture("MAX_LAT_MS_(?<ms>[0-9]+)")? | .ms // empty' "$file" | sort -n | tail -n 1)"
  if [ -z "$max_lat" ]; then
    echo "FAIL $label missing MAX_LAT_MS"
    failures=$((failures+1))
  elif [ "$max_lat" -le "$FANOUT_MAX_LAT_MS" ]; then
    echo "OK   $label max_observed_live_latency_ms=$max_lat <= $FANOUT_MAX_LAT_MS"
  else
    echo "FAIL $label max_observed_live_latency_ms=$max_lat > $FANOUT_MAX_LAT_MS"
    failures=$((failures+1))
  fi
}

echo "S3 PTS evidence verifier"
echo "source: $JSONL"
if [ -f "$DETAILS" ]; then
  echo "report_details: $DETAILS"
  echo "note: PTS sampling logs are diagnostic rows; report-details is used for sampler counts/success/RT when present."
else
  echo "report_details: missing; falling back to sampling-log counts/RT"
fi
echo "expected: bids=$EXPECTED_BIDS ws=$EXPECTED_WS readers=$EXPECTED_READERS exact_counts=$REQUIRE_EXACT_COUNTS"

aux_missing_severity="FAIL"
if [ "$REQUIRE_EXACT_COUNTS" != "1" ]; then
  aux_missing_severity="WARN"
fi

assert_count "S3 POST accepted-update bid" "$EXPECTED_BIDS"
assert_all_success "S3 POST accepted-update bid"
assert_p99_lte "S3 POST accepted-update bid" "$BID_P99_MAX_MS"

assert_count "S3 viewer join snapshot" "$EXPECTED_WS" "$aux_missing_severity"
assert_all_success "S3 viewer join snapshot"
assert_p99_lte "S3 viewer join snapshot" "$JOIN_SEGMENT_P99_MAX_MS" "$aux_missing_severity"

assert_count "S3 POST WS ticket" "$EXPECTED_WS"
assert_all_success "S3 POST WS ticket"
assert_p99_lte "S3 POST WS ticket" "$JOIN_SEGMENT_P99_MAX_MS"

assert_count "S3 WS handshake complete" "$EXPECTED_WS"
assert_all_success "S3 WS handshake complete"
assert_p99_lte "S3 WS handshake complete" "$JOIN_SEGMENT_P99_MAX_MS"

assert_count "S3 WS first snapshot/business message" "$EXPECTED_WS"
assert_all_success "S3 WS first snapshot/business message"
assert_p99_lte "S3 WS first snapshot/business message" "$JOIN_SEGMENT_P99_MAX_MS"

assert_count "S3 live fanout receive" "$EXPECTED_WS"
assert_all_success "S3 live fanout receive"
assert_fanout_response

for label in "S3 GET auction snapshot" "S3 GET auction leaderboard" "S3 GET my bid history"; do
  assert_count "$label" "$EXPECTED_READERS" "$aux_missing_severity"
  assert_all_success "$label"
  assert_p99_lte "$label" "$READ_P99_MAX_MS" "$aux_missing_severity"
done

if [ "$failures" -ne 0 ]; then
  echo "S3 verdict: FAIL ($failures failed checks)"
  exit 1
fi

echo "S3 verdict: PASS"
