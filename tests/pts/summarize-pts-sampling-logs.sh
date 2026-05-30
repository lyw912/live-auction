#!/usr/bin/env bash
set -euo pipefail

JSONL="${1:?usage: bash tests/pts/summarize-pts-sampling-logs.sh sampling-logs.jsonl [EXPECTED_COUNT]}"
EXPECTED_COUNT="${2:-${EXPECTED_COUNT:-}}"
FIELD="${FIELD:-elapsedTime}"
SAMPLER_ID="${SAMPLER_ID:-}"
SAMPLER_LABEL="${SAMPLER_LABEL:-}"

command -v jq >/dev/null 2>&1 || {
  echo "jq is required" >&2
  exit 127
}

if [ ! -f "$JSONL" ]; then
  echo "sampling log not found: $JSONL" >&2
  exit 2
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

values="$tmp_dir/values.txt"
meta="$tmp_dir/meta.tsv"

jq -r \
  --arg field "$FIELD" \
  --arg sampler_id "$SAMPLER_ID" \
  --arg sampler_label "$SAMPLER_LABEL" \
  'select(($sampler_id == "" or ((.samplerId // "") | tostring) == $sampler_id)
          and ($sampler_label == "" or (.samplerLabel // "") == $sampler_label))
   | .[$field] // empty
   | select(type == "number" or type == "string")
   | tostring' "$JSONL" | sort -n > "$values"

jq -r \
  --arg sampler_id "$SAMPLER_ID" \
  --arg sampler_label "$SAMPLER_LABEL" \
  'select(($sampler_id == "" or ((.samplerId // "") | tostring) == $sampler_id)
          and ($sampler_label == "" or (.samplerLabel // "") == $sampler_label))
   | [
       (.success // ""),
       (.responseCode // ""),
       (.samplerId // ""),
       (.samplerLabel // ""),
       (.startTimeTS // ""),
       (.endTimeTS // "")
     ] | @tsv' "$JSONL" > "$meta"

count="$(wc -l < "$values" | tr -d ' ')"
if [ "$count" -eq 0 ]; then
  echo "no samples matched field=$FIELD sampler_id=${SAMPLER_ID:-*} sampler_label=${SAMPLER_LABEL:-*}" >&2
  exit 3
fi

coverage="UNKNOWN"
if [ -n "$EXPECTED_COUNT" ]; then
  if [ "$count" -eq "$EXPECTED_COUNT" ]; then
    coverage="FULL"
  else
    coverage="SAMPLE_ONLY"
  fi
fi

echo "PTS sampling-log latency summary"
echo "source: $JSONL"
echo "field: $FIELD"
echo "sampler_id: ${SAMPLER_ID:-*}"
echo "sampler_label: ${SAMPLER_LABEL:-*}"
echo "samples: $count"
if [ -n "$EXPECTED_COUNT" ]; then
  echo "expected: $EXPECTED_COUNT"
fi
echo "coverage: $coverage"

awk '
  function ceil(x) { return x == int(x) ? x : int(x) + 1 }
  function pct(p) {
    idx = ceil(p * n)
    if (idx < 1) idx = 1
    if (idx > n) idx = n
    return v[idx]
  }
  { n++; v[n] = $1 + 0; sum += v[n] }
  END {
    if (n == 0) exit 1
    printf "min_ms: %.3f\n", v[1]
    printf "avg_ms: %.3f\n", sum / n
    printf "p50_ms: %.3f\n", pct(0.50)
    printf "p90_ms: %.3f\n", pct(0.90)
    printf "p95_ms: %.3f\n", pct(0.95)
    printf "p99_ms: %.3f\n", pct(0.99)
    printf "max_ms: %.3f\n", v[n]
  }
' "$values"

echo "response_codes:"
awk -F '\t' '
  {
    code = $2 == "" ? "<empty>" : $2
    codes[code]++
    success = $1 == "" ? "<empty>" : $1
    successes[success]++
  }
  END {
    for (code in codes) printf "- %s: %d\n", code, codes[code]
    print "success:"
    for (success in successes) printf "- %s: %d\n", success, successes[success]
  }
' "$meta"

awk -F '\t' '
  NR == 1 || ($5 != "" && $5 < first) { first = $5 }
  NR == 1 || ($6 != "" && $6 > last) { last = $6 }
  END {
    if (first != "" && last != "") {
      printf "window_start_ts_ms: %s\n", first
      printf "window_end_ts_ms: %s\n", last
      printf "window_span_ms: %d\n", last - first
    }
  }
' "$meta"
