#!/usr/bin/env bash
set -euo pipefail

INPUT="${1:?usage: bash tests/pts/review-s1-pts-run.sh <report-id|label> [evidence-dir]}"
EVIDENCE_DIR="${2:-docs/perf/pts/evidence/incoming/$INPUT}"
SAMPLER_LABEL="${SAMPLER_LABEL:-出价决策 bid-decision}"
EXPECTED_COUNT="${EXPECTED_COUNT:-1000}"

JSONL="$EVIDENCE_DIR/pts-sampling-logs/sampling-logs.jsonl"
METRICS="$EVIDENCE_DIR/metrics.prom"
GATES="$EVIDENCE_DIR/l4b-invariant-gates.tsv"
DETAILS="$EVIDENCE_DIR/pts-report-details.json"
OUT="$EVIDENCE_DIR/s1-review.md"

command -v jq >/dev/null 2>&1 || {
  echo "jq is required" >&2
  exit 127
}

if [ ! -f "$JSONL" ]; then
  echo "sampling log not found: $JSONL" >&2
  echo "Fetch it first, preferably with 100% PTS sampling for judge-facing S1:" >&2
  echo "  PAGE_SIZE=100 bash tests/pts/fetch-pts-sampling-logs.sh $INPUT" >&2
  exit 2
fi

if [ ! -f "$DETAILS" ] && command -v aliyun >/dev/null 2>&1; then
  aliyun pts get-jmeter-report-details --report-id "$INPUT" > "$DETAILS" 2>/dev/null || rm -f "$DETAILS"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

sample_filter='select(.samplerLabel == $label)'

metric_percentiles() {
  local field="$1"
  local out="$tmp_dir/${field}.values"
  jq -r --arg label "$SAMPLER_LABEL" --arg field "$field" \
    "$sample_filter | .[\$field] // empty | select(type == \"number\" or type == \"string\") | tostring" \
    "$JSONL" | sort -n > "$out"
  awk -v field="$field" '
    function ceil(x) { return x == int(x) ? x : int(x) + 1 }
    function pct(p) {
      idx = ceil(p * n)
      if (idx < 1) idx = 1
      if (idx > n) idx = n
      return v[idx]
    }
    { n++; v[n] = $1 + 0; sum += v[n] }
    END {
      if (n == 0) {
        printf "| %s | 0 | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |\n", field
        exit
      }
      printf "| %s | %d | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f |\n",
        field, n, v[1], sum / n, pct(0.50), pct(0.75), pct(0.90), pct(0.95), pct(0.99), v[n], v[n]
    }
  ' "$out"
}

dist_table() {
  local title="$1"
  local jq_expr="$2"
  echo "### $title"
  echo
  echo "| Value | Count |"
  echo "|---|---:|"
  jq -r --arg label "$SAMPLER_LABEL" \
    "def body: ((.responseDataAsString // \"{}\") | fromjson? // {});
     $sample_filter | $jq_expr" "$JSONL" |
    sort | uniq -c | sort -nr |
    awk '{ count=$1; $1=""; sub(/^ /,""); printf "| %s | %d |\n", $0, count }'
  echo
}

span_value() {
  local jq_expr="$1"
  jq -sr --arg label "$SAMPLER_LABEL" \
    "def body: ((.responseDataAsString // \"{}\") | fromjson? // {});
     map(select(.samplerLabel == \$label) | $jq_expr | select(. != null)) | if length == 0 then \"n/a\" else ((max - min) | tostring) end" \
    "$JSONL"
}

range_value() {
  local jq_expr="$1"
  jq -sr --arg label "$SAMPLER_LABEL" \
    "def body: ((.responseDataAsString // \"{}\") | fromjson? // {});
     map(select(.samplerLabel == \$label) | $jq_expr | select(. != null)) |
     if length == 0 then \"n/a\" else ((min|tostring) + \"..\" + (max|tostring)) end" \
    "$JSONL"
}

scalar_count() {
  jq -sr --arg label "$SAMPLER_LABEL" 'map(select(.samplerLabel == $label)) | length' "$JSONL"
}

unique_count() {
  local jq_expr="$1"
  jq -sr --arg label "$SAMPLER_LABEL" \
    "def body: ((.responseDataAsString // \"{}\") | fromjson? // {});
     map(select(.samplerLabel == \$label) | $jq_expr | select(. != null and . != \"\")) | unique | length" \
    "$JSONL"
}

engine_seq_stats() {
  jq -sr --arg label "$SAMPLER_LABEL" \
    "def body: ((.responseDataAsString // \"{}\") | fromjson? // {});
     map(select(.samplerLabel == \$label) | body.engine_seq? | select(. != null) | tonumber) | sort as \$s |
     (\$s | unique) as \$u |
     if (\$s | length) == 0 then
       \"count=0 min=n/a max=n/a unique=0 duplicate_count=0 missing_count=n/a\"
     else
       \"count=\((\$s|length)) min=\(\$s[0]) max=\(\$s[-1]) unique=\((\$u|length)) duplicate_count=\((\$s|length)-(\$u|length)) missing_count=\((\$s[-1]-\$s[0]+1)-(\$u|length))\"
     end" "$JSONL"
}

prom_line_value() {
  local regex="$1"
  if [ -f "$METRICS" ]; then
    awk -v re="$regex" '$0 ~ re { print $2; found=1 } END { if (!found) print "n/a" }' "$METRICS" | tail -n 1
  else
    echo "n/a"
  fi
}

prom_bucket_table() {
  local metric="$1"
  local label_regex="$2"
  if [ ! -f "$METRICS" ]; then
    echo "| unavailable | n/a |"
    return
  fi
  awk -v metric="$metric" -v label_re="$label_regex" '
    $1 ~ ("^" metric "_bucket\\{") && $0 ~ label_re {
      le=$0
      sub(/^.*le="/, "", le)
      sub(/".*$/, "", le)
      printf "| <= %s s | %s |\n", le, $2
    }
  ' "$METRICS"
}

report_consistency="not checked"
pts_api_table=""
if [ -f "$DETAILS" ]; then
  pts_api_table="$(
    jq -r --arg label "$SAMPLER_LABEL" '
      .SamplerMetricsList[]? | select(.ApiName == $label) |
      [.AllCount, .MinRt, .AvgRt, .MaxRt, .Seg75Rt, .Seg90Rt, .Seg99Rt, .SuccessRateReq] | @tsv
    ' "$DETAILS" | awk -F '\t' '
      NR == 1 {
        printf "| AllCount | MinRt | AvgRt | MaxRt | Seg75Rt | Seg90Rt | Seg99Rt | SuccessRateReq |\n"
        printf "|---:|---:|---:|---:|---:|---:|---:|---:|\n"
        printf "| %s | %s | %s | %s | %s | %s | %s | %s |\n", $1,$2,$3,$4,$5,$6,$7,$8
      }
    '
  )"
  report_consistency="$(
    jq -r --arg label "$SAMPLER_LABEL" '
      .SamplerMetricsList[]? | select(.ApiName == $label) |
      if ((.Seg99Rt // 0) > (.MaxRt // 0)) then
        "INVALID: Seg99Rt=\(.Seg99Rt) > MaxRt=\(.MaxRt); do not cite API/report p99 for this sampler."
      else
        "OK: Seg99Rt <= MaxRt for this sampler."
      end
    ' "$DETAILS" | head -n 1
  )"
fi

sample_count="$(scalar_count)"
unique_bid_ids="$(unique_count '((.requestData // "{}") | fromjson? // {} | .client_bid_id)')"
unique_response_bid_ids="$(unique_count '(body.bid_id)')"
durability_kafka_acked_count="$(jq -sr --arg label "$SAMPLER_LABEL" 'def body: ((.responseDataAsString // "{}") | fromjson? // {}); map(select(.samplerLabel == $label and body.durability_status == "KAFKA_ACKED")) | length' "$JSONL")"
durability_engine_durable_count="$(jq -sr --arg label "$SAMPLER_LABEL" 'def body: ((.responseDataAsString // "{}") | fromjson? // {}); map(select(.samplerLabel == $label and body.durability_status == "ENGINE_DURABLE")) | length' "$JSONL")"
start_span_ms="$(span_value '.startTimeTS')"
end_span_ms="$(span_value '.endTimeTS')"
server_time_span_ms="$(span_value '(body.server_time_ms)')"
start_range="$(range_value '.startTimeTS')"
server_time_range="$(range_value '(body.server_time_ms)')"
engine_stats="$(engine_seq_stats)"
http_post_count="$(prom_line_value '^http_request_total\\{method="POST",path="/api/auctions/\\{id\\}/bids",status="200"\\}')"
redis_lua_count="$(prom_line_value '^redis_lua_script_total\\{outcome="ok",script="bid_redis_ledger"\\}')"
git_sha="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
dirty_tree="$(git status --short 2>/dev/null | wc -l | tr -d ' ')"
generated_at="$(date -Is)"

{
  echo "# S1 PTS Evidence Review: $INPUT"
  echo
  echo "| Field | Value |"
  echo "|---|---|"
  echo "| Generated at | $generated_at |"
  echo "| Git SHA | $git_sha |"
  echo "| Dirty tree entries | $dirty_tree |"
  echo "| Evidence dir | $EVIDENCE_DIR |"
  echo "| Sampling JSONL | $JSONL |"
  echo "| Sampler | $SAMPLER_LABEL |"
  echo "| Expected S1 bids | $EXPECTED_COUNT |"
  echo "| Default response profile | kafka_ack: expects KAFKA_ACKED >= 99%, bounded ENGINE_DURABLE fallback, and post-run convergence |"
  echo
  echo "## Verdict Inputs"
  echo
  echo "| Check | Value | Interpretation |"
  echo "|---|---:|---|"
  echo "| PTS sampling rows | $sample_count | Must equal expected count for 100% sampling evidence. |"
  echo "| Unique request client_bid_id | $unique_bid_ids | Detects CSV split/idempotency replay contamination. |"
  echo "| Unique response bid_id | $unique_response_bid_ids | Detects duplicate persisted bid identities. |"
  echo "| Server POST /bids count | $http_post_count | Independent server-side request count from Prometheus. |"
  echo "| Redis Lua bid ledger count | $redis_lua_count | Independent hot-engine execution count. |"
  echo "| PTS startTimeTS span ms | $start_span_ms | Load-generator send-start span; use this for offered-burst timing. |"
  echo "| PTS endTimeTS span ms | $end_span_ms | Client-observed completion span. |"
  echo "| Response server_time_ms span ms | $server_time_span_ms | Server decision timestamp span from response bodies. |"
  echo "| PTS startTimeTS range | $start_range | Raw millisecond range. |"
  echo "| Response server_time_ms range | $server_time_range | Raw millisecond range. |"
  echo "| Engine seq stats | $engine_stats | Must be complete, unique, and gap-free. |"
  echo
  echo "## Exact PTS Sampling Percentiles"
  echo
  echo "Nearest-rank formula over sorted 100% sampling rows: rank = ceil(phi * N). These values are calculated from raw JMeter/PTS sampling fields, not from the report page."
  echo
  echo "| Field | N | Min ms | Avg ms | P50 ms | P75 ms | P90 ms | P95 ms | P99 ms | Max ms | Upper bound ms |"
  echo "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"
  metric_percentiles "elapsedTime"
  metric_percentiles "latency"
  metric_percentiles "connectTime"
  echo
  dist_table "HTTP / Assertion Distribution" '((.responseCode // "<empty>") + " success=" + ((.success // "")|tostring) + " message=" + (.responseMessage // "<empty>"))'
  dist_table "Business Result Distribution" '(body.result // "<missing>")'
  dist_table "Decision Status Distribution" '(body.decision_status // "<missing>")'
  dist_table "Durability Status Distribution" '(body.durability_status // "<missing>")'
  dist_table "Settlement Status Distribution" '(body.settlement_status // "<missing>")'
  echo "## PTS Report/API Consistency"
  echo
  if [ -n "$pts_api_table" ]; then
    echo "$pts_api_table"
  else
    echo "PTS report details were not fetched."
  fi
  echo
  echo "$report_consistency"
  echo
  echo "## Server Prometheus Cross-Checks"
  echo
  echo "Prometheus histograms are bucketed counters. This report uses bucket upper bounds and counts; it does not fabricate an exact server p99 from coarse buckets."
  echo
  echo "### HTTP /api/auctions/{id}/bids latency buckets"
  echo
  echo "| Bucket | Cumulative count |"
  echo "|---|---:|"
  prom_bucket_table "http_request_latency_seconds" 'method="POST",path="/api/auctions/\{id\}/bids"'
  echo
  echo "### Gateway total stage buckets"
  echo
  echo "| Bucket | Cumulative count |"
  echo "|---|---:|"
  prom_bucket_table "auction_bid_gateway_stage_seconds" 'stage="total"'
  echo
  echo "### Gateway redis_engine stage buckets"
  echo
  echo "| Bucket | Cumulative count |"
  echo "|---|---:|"
  prom_bucket_table "auction_bid_gateway_stage_seconds" 'stage="redis_engine"'
  echo
  echo "### Redis Lua bid_redis_ledger buckets"
  echo
  echo "| Bucket | Cumulative count |"
  echo "|---|---:|"
  prom_bucket_table "redis_lua_script_latency_seconds" 'script="bid_redis_ledger"'
  echo
  echo "## Correctness Gates"
  echo
  if [ -f "$GATES" ]; then
    echo "| Status | Count |"
    echo "|---|---:|"
    awk -F '\t' '{ counts[$3]++ } END { for (s in counts) printf "| %s | %d |\n", s, counts[s] }' "$GATES"
    echo
    non_pass="$(awk -F '\t' '$3 != "PASS" { print }' "$GATES")"
    if [ -n "$non_pass" ]; then
      echo "Non-PASS gates:"
      echo
      echo '```text'
      echo "$non_pass"
      echo '```'
    else
      echo "All invariant gates are PASS."
    fi
  else
    echo "Correctness gate file not found: $GATES"
  fi
  echo
  echo "## Classification Rule"
  echo
  echo "Default kafka_ack rule: a run is S1 CURRENT_PASS only if sampling rows, unique client_bid_id, server POST count, Redis Lua count, engine_seq continuity, settlement/outbox/Kafka gates, KAFKA_ACKED >= 99%, bounded ENGINE_DURABLE fallback, and p99 <= 60 ms all pass. Current durability counts: KAFKA_ACKED=$durability_kafka_acked_count, ENGINE_DURABLE=$durability_engine_durable_count."
  echo
  echo "Explicit redis_aof diagnostic rule: if the server was intentionally run with BID_ENGINE_RESPONSE_DURABILITY=redis_aof, require ENGINE_DURABLE decisions and p99 <= 50 ms, and label the evidence as redis_aof low-latency rather than default kafka_ack."
  echo
  echo "If the PTS report/API p99 violates Seg99Rt <= MaxRt, cite the recomputed 100% sampling percentile and mark the report field as invalid."
} > "$OUT"

echo "Wrote $OUT"
