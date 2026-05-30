#!/usr/bin/env bash
set -euo pipefail

REPORT_ID="${1:?usage: bash tests/pts/fetch-pts-sampling-logs.sh REPORT_ID [OUT_DIR]}"
OUT_DIR="${2:-docs/perf/pts/evidence/incoming/$REPORT_ID/pts-sampling-logs}"
PAGE_SIZE="${PAGE_SIZE:-100}"
PAGE_START="${PAGE_START:-1}"
MAX_PAGES="${MAX_PAGES:-100000}"
PTS_REGION="${PTS_REGION:-cn-heyuan}"
PTS_ENDPOINT="${PTS_ENDPOINT:-pts.aliyuncs.com}"

mkdir -p "$OUT_DIR/pages"

command -v aliyun >/dev/null 2>&1 || {
  echo "aliyun CLI is not installed or not in PATH" >&2
  exit 127
}
command -v jq >/dev/null 2>&1 || {
  echo "jq is required" >&2
  exit 127
}

if ! aliyun pts get-jmeter-sampling-logs --help >/dev/null 2>&1; then
  echo "aliyun PTS plugin is required. Install it with:" >&2
  echo "  aliyun plugin install --name aliyun-cli-pts" >&2
  exit 127
fi

csv="$OUT_DIR/sampling-logs.csv"
jsonl="$OUT_DIR/sampling-logs.jsonl"
: > "$jsonl"
printf 'startTime,endTime,elapsedTime,latency,connectTime,success,responseCode,httpMethod,samplerId,samplerLabel,threadName,url,responseMessage,requestByteCount,responseByteCount\n' > "$csv"

page="$PAGE_START"
fetched=0
while [ "$page" -lt "$((PAGE_START + MAX_PAGES))" ]; do
  raw="$OUT_DIR/pages/page-$page.json"
  echo "[pts] fetching report=$REPORT_ID page=$page size=$PAGE_SIZE"
  aliyun pts get-jmeter-sampling-logs \
    --endpoint "$PTS_ENDPOINT" \
    --region "$PTS_REGION" \
    --report-id "$REPORT_ID" \
    --page-number "$page" \
    --page-size "$PAGE_SIZE" > "$raw"

  count="$(jq '.SampleResults | length' "$raw")"
  total="$(jq -r '.TotalCount // 0' "$raw")"
  if [ "$count" = "0" ]; then
    break
  fi

  jq -r '.SampleResults[] | if type == "string" then fromjson else . end | @json' "$raw" >> "$jsonl"
  jq -r '.SampleResults[] | if type == "string" then fromjson else . end |
    [
      (.startTimeTS // .startTime // ""),
      (.endTimeTS // .endTime // ""),
      (.elapsedTime // ""),
      (.latency // ""),
      (.connectTime // ""),
      (.success // ""),
      (.responseCode // ""),
      (.httpMethod // ""),
      (.samplerId // ""),
      (.samplerLabel // ""),
      (.threadName // ""),
      (.url // ""),
      (.responseMessage // ""),
      (.requestByteCount // ""),
      (.responseByteCount // "")
    ] | @csv' "$raw" >> "$csv"

  fetched="$((fetched + count))"
  if [ "$fetched" -ge "$total" ] && [ "$total" -gt 0 ]; then
    break
  fi
  page="$((page + 1))"
done

echo "[pts] wrote:"
echo "- $jsonl"
echo "- $csv"
