# S3 Fanout Diagnosis And Judge Defense

> Status: current diagnostic note, 2026-06-02.
> Scope: local S3 1000-WS fanout runs, the 59.6s vs 22ms discrepancy, and
> judge-facing answers. This document is evidence interpretation, not a new
> workload definition.

## 1. Executive Verdict

S3 currently has one defensible local 1000-WS live fanout result and one useful
harness-failure result:

| Claim | Current verdict | Evidence |
|---|---|---|
| 1000 single-room viewers can stay connected for the 60s local S3 window | PASS | `s3-local-scale-1000-liveonly-20260602T2303`: `s3_viewer_connected=1000`, `ws_session_duration p99=60150ms` |
| Live fanout M2 p99 <= 1s at 1000 WS | PASS | same run: `s3_fanout_latency_ms p99=22ms`, p95=14ms, max=51ms, 276000 fanout samples |
| Every viewer got at least one live update | PASS | same run: `s3_viewer_received_session=1`, 1000 check passes |
| Viewer connection errors | PASS | same run: `s3_viewer_errors=0` |
| 2000 WS local capacity | FAIL / unproven | `s3-local-scale-2000-20260602T2305` has only pre-run metrics and events; no completed summary |
| 10000 WS headline | UNPROVEN | Requires PTS headline or completed local 10k hold + Grafana resource evidence |

The honest judge-facing statement:

> "S3 is the single-room fanout workload, not the bid-decision workload. In the
> current local 1000-WS live-only run, 1000 viewers stayed connected, 301 accepted
> updates produced 276000 client receive samples, fanout p99 was 22ms, and viewer
> error count was zero. This proves the official 1000+ single-room online bonus
> and second-level price synchronization at the 1000-WS scale. The attempted
> 2000-WS run is not cited as pass; it is capacity-ceiling evidence to be
> followed by PTS multi-IP or gateway sharding."

## 2. Why 59.6s And 22ms Do Not Conflict

Both runs used 1000 WebSocket viewers, but they did not measure the same event
population.

`s3-local-scale-1000-20260602T2300` used the old measurement rule:

```text
if a message has published_at_ms:
  fanout_latency = client_receive_time - published_at_ms
```

That rule accidentally included history/recovery messages. A new or reconnecting
viewer may receive older public events that were published before the viewer was
online. Those messages can legitimately be tens of seconds old. They are useful
for reconnect/snapshot correctness, but they are not live fanout latency.

`s3-local-scale-1000-liveonly-20260602T2303` uses the corrected rule:

```text
connectedAtMs = client time when the WS opens
if published_at_ms >= connectedAtMs:
  measure as live fanout
else:
  ignore for M2
```

That aligns with the S3/M2 definition:

```text
server publishes a new price event while viewers are online
  -> online viewers receive that event
```

So the old 59.6s run is not a contradictory performance result. It is a harness
diagnostic that found a measurement contamination.

## 3. What S3 Actually Proves

S3 answers one business question:

> "In one hot live room, when a valid accepted bid changes the price, how long
> until online viewers see that new price?"

It deliberately decouples two variables:

| Variable | S3 treatment | Why |
|---|---|---|
| Viewer connection count | the independent variable, e.g. 1000 -> 2000 -> 10000 | fanout cost scales with subscribers |
| Accepted update rate | low fixed source, current local run about 5/s | avoids mixing bid-engine contention with WS downlink |

Do not use S3 to claim bid-engine throughput. S1/S2 measure bid decision
goodput and p99. S3 measures accepted-update fanout to viewers.

## 4. Evidence Table For Judge Report

| Metric | Current value | Source | Judge wording |
|---|---:|---|---|
| Online viewers | 1000 | `s3_viewer_connected` | "1000 single-room WS connections were established" |
| Live update source | 301 accepted updates | `s3_bid_accepted_updates` | "A real bid source produced price changes; clients were not idle" |
| Client receive samples | 276000 | `s3_fanout_samples` | "p99 is across many viewer-event receives, not one lucky sample" |
| Fanout p99 | 22ms | `s3_fanout_latency_ms p99` | "well below the 1s same-region synchronization target" |
| Fanout max | 51ms | `s3_fanout_latency_ms max` | "no long live fanout tail in this run" |
| Viewer errors | 0 | `s3_viewer_errors` | "no unintended viewer close/error in the completed run" |
| WS connect p99 | 150ms | `ws_connecting p99` | "join handshake stayed bounded for this local run" |

## 5. Judge Grill

**Q: You have one 1000-WS run at 59.6s and another at 22ms. Which one is true?**

A: They answer different measurement populations. The 59.6s result counted old
history/recovery messages as live fanout. After fixing the harness to count only
messages published after the viewer was online, the live fanout p99 was 22ms.
We cite the corrected live-only run and keep the earlier run as a harness bug
record.

**Q: Are you hiding a user-visible problem by excluding history messages?**

A: No. History/recovery is a real user concern, but it is not S3/M2. It belongs
to reconnect/snapshot recovery, where the metric is time-to-current-state and no
sequence gaps. S3 measures online viewers receiving new price events.

**Q: Does 1000 WS prove 10000 WS?**

A: No. It proves the official 1000+ single-room baseline/bonus scale locally.
The 10000 headline still needs either PTS multi-IP evidence or a completed local
10k hold with Grafana fd/goroutine/RSS panels. The 2000 local crash is treated
as capacity-boundary evidence, not as a success.

**Q: Why are accepted updates only about 5/s?**

A: Accepted update rate is intentionally low and fixed because S3 isolates
fanout cost. Fanout volume is accepted_updates_per_second x viewers. Bid
decision capacity is measured in S1/S2 with accept+reject decisions.

**Q: What would you do if the judge asks for scale-out?**

A: Say one node is one shard. Horizontal path is room-sharded WebSocket
gateways, pub/sub fanout per room shard, and Kafka partitioning by auction. The
single-auction sequencer remains ordered; the fanout plane scales by rooms and
gateways.

## 6. 2026-06-03 PTS HFH3X74G Incident

`HFH3X74G` is classified as `CURRENT_FAILING / HARNESS_DIAGNOSTIC`, not as an
S3 pass.

Observed facts:

| Signal | Value | Interpretation |
|---|---:|---|
| Backend WS connections | 4985 | PTS did reach the backend at near-full S3 viewer scale |
| Backend bid requests | 995 | Bid source reached the backend |
| PTS sampling logs | dozens of samples per label | sampling logs are diagnostic only; not enough for exact-count proof |
| `S3 live fanout receive` | `MAX_LAT_MS_132` | online-viewer live fanout looked healthy in sampled clients |
| `S3 WS upgrade to first message` | p99 2394ms | user-visible realtime-ready tail failed the 1s join target |
| `/api/auctions/{id}` requests | about 1.75M | old JMX polluted the run with per-viewer 250ms snapshot polling |
| `auction_ws_recover_total{result="snapshot_db"}` | 4985 | first WS messages were served through DB snapshot recovery, not Redis hot snapshot |

User-experience interpretation:

- HTTP `S3 viewer join snapshot` maps to the first visible room state. It was
  fast enough in this run.
- `S3 WS first snapshot/business message` maps to realtime channel readiness. A
  2.4s p99 means some viewers wait seconds before live updates are guaranteed.
- `S3 live fanout receive` maps to already-online viewers receiving a new
  accepted price update. That remains the M2 headline metric.

Industry bar used for interpretation:

- Google/web.dev recommends INP <= 200ms for good interaction responsiveness:
  <https://web.dev/articles/optimize-inp>.
- NN/g's response-time guidance treats around 1s as the boundary where users
  keep flow but notice waiting:
  <https://www.nngroup.com/topic/response-time>.
- Centrifugo's public realtime benchmark reports 200ms p99 delivery latency at
  very high fanout scale, showing that sub-second realtime delivery is a
  conservative bar, not an aggressive one:
  <https://centrifugal.dev/blog/2020/02/10/million-connections-with-centrifugo>.

Corrective changes made after the incident:

1. S3 JMX v6 separates `S3 WS handshake complete`, `S3 WS first
   snapshot/business message`, and `S3 live fanout receive`.
2. S3 JMX v6 removed the per-WebSocket 250ms `/api/auctions/auc_live` polling
   loop from the fanout sampler. Read interference is only the explicit reader
   thread group.
3. S3 prepare now pre-seeds `auction:auc_live:snapshot` with a real
   `event_type=snapshot` Redis payload.
4. The realtime server now treats `last_seq=0` initial joins as snapshot joins,
   not history replay, and rejects Redis `auction:<id>:snapshot` values that are
   actually latest-event envelopes.
5. `ws_reconnect/ws_recovered` audit writes moved off the first-message path via
   a bounded async queue.
6. `auction_ws_join_stage_seconds{stage=...}` was added so future runs can
   attribute slow first-message latency to ticket, room access, accept,
   recovery, or first write.

Required post-fix smoke:

```bash
bash tests/pts/prepare-s3-room-fanout-pressure.sh
# Upload:
# - tests/pts/S3-room-fanout/s3-live-fanout-smoke-30vu-single-branch-20ws-5bid-5read.jmx
# - docs/perf/pts/s3-mixed-smoke-30-sessions.csv
# PTS: 30 VU, 1 IP, 1 minute, 100% sampling.
```

Pass conditions for the smoke:

- `GetJMeterReportDetails` has the expected `SamplerMetricsList.AllCount`,
  `SuccessRateReq=100`, and bounded `Seg99Rt` for every S3 sampler. This is the
  count/RT source of truth.
- `S3 WS handshake complete` succeeds.
- `S3 WS first snapshot/business message` p99 <= 1000ms.
- `S3 live fanout receive` sampling-log response contains
  `S3_V6_LIVE_FANOUT_OK...WS_ONLY`; sampling logs are used here for response
  marker forensics, not exact sampler coverage.
- Server metrics show `auction_snapshot_source_total{source="redis"}` for the
  initial WS cohort, not `source="db"`.

`VAH7X7CG` confirmed the final evidence split: report details showed all ten S3
samplers ran with expected counts and 100% success, while
`GetJMeterSamplingLogs` returned rows only for sampler ids 0,4,5,6,7. Treat this
as a sampling-log retrieval limitation, not a JMX execution failure.

Handshake note from the same run: `S3 WS handshake complete` p99 was 596ms, which
is acceptable for smoke but not excellent. Server metrics did not support a
backend-accept bottleneck: `auction_ws_join_stage_seconds{stage="total"}` was
under 10ms for all 20 joins, `accept` was sub-millisecond, Redis ticket consume
averaged about 2ms, and Redis snapshot recovery averaged about 1.5ms. The
current interpretation is therefore client-observed PTS/JMeter/network timing,
not business fanout latency. The S3 v7 JMX records `SETUP_MS` and `HANDSHAKE_MS`
inside the handshake response marker and reuses one Java `HttpClient` per PTS
engine to remove client construction noise from the sampler.

## 7. PTS Live-Only Fanout Evidence: `XWLAX70G`

`XWLAX70G` is the current PTS `S3-live-only-fanout` evidence.

PTS console API-list summary:

| Sampler | Count | Success | Assertion | Average RT |
|---|---:|---:|---:|---:|
| `S3 POST accepted-update bid` | 100 | 100% | 100% | 5.48 ms |
| `S3 viewer join snapshot` | 2994 | 100% | 100% | 123 ms |
| `S3 POST WS ticket` | 2994 | 100% | 100% | 40.1 ms |
| `S3 WS handshake complete` | 2994 | 100% | 100% | 912 ms |
| `S3 WS first snapshot/business message` | 2994 | 100% | 100% | 112 ms |
| `S3 live fanout receive` | 2994 | 100% | 100% | 59.8 ms |
| `S3 WS close` | 2994 | 100% | 100% | 112 ms |

PTS total sampler requests:

```text
100 + 2994 * 6 = 18064
```

This is expected. JMeter counts sampler executions; it does not count each
WebSocket message as a separate request.

Service-side confirmation:

| Signal | Value | Meaning |
|---|---:|---|
| accepted bids | 100/100 | all update-source bids became price updates |
| `auc_live.seq` | 100 | auction sequence advanced for each accepted update |
| Kafka lag | 0 | settlement consumer drained |
| Outbox pending | 0 | publication plane drained |
| `auction_ws_recover_total{snapshot_redis}` | 2994 | viewers joined via Redis hot snapshot |
| `auction_ws_publish_subscribers_sum` | 299400 | `100 accepted publishes * 2994 subscribers` |
| WS send queue depth | 0 | no observed queue buildup |
| WS connections after run | 0 | clean close after PTS run |
| Redis rejected/evicted/blocked | 0 | no Redis connection or memory failure |

Important count wording:

- Do not say "the database has 299400 rows." It does not. PostgreSQL contains
  100 accepted bids and 100 outbox records.
- `299400` is the service metric for subscriber-delivery surface:
  every accepted publish was sent to each online subscriber.
- PTS `S3 live fanout receive=2994` is one receive sampler per viewer, not the
  WebSocket message count.

Sampling caveat:

- The run used 1% sampling. Sampling logs are therefore response-marker
  forensics, not exact-count proof.
- Retrieved logs sampled 38 `S3 live fanout receive` rows. Every sampled row
  reported `LIVE_MESSAGES_100`, `LIVE_SEQS_100`, and `FINAL_SEQ_100`.
- Sampled `MAX_LAT_MS` values were roughly `25-169 ms`.

Why p99 stayed close to the 7-update mixed run:

> "This is not evidence that the pressure leaked away. The backend metrics show
> the fanout surface was 299400 subscriber-deliveries, versus 20965 in the
> 7-update mixed run. The similar PTS p99 means this 3000-viewer / 100-update
> point did not saturate the current fanout path; queue depth stayed zero and
> every sampled viewer received 100 live messages. It also means we should not
> present PTS sampler p99 as a full per-frame latency histogram. The correct
> evidence chain is PTS sampler success + sampled `MAX_LAT_MS` markers + service
> publish-subscriber and queue/backlog metrics."

## 8. PTS Mixed Final-Burst Evidence: `20L8X79G`

`20L8X79G` is the current PTS `S3-mixed-final-burst` integration evidence, not
the clean `S3-live-only-fanout` baseline.

PTS console API-list export:

| Sampler | Count | Success | Assertion | Average RT |
|---|---:|---:|---:|---:|
| `S3 POST accepted-update bid` | 998 | 100% | 100% | 17.5 ms |
| `S3 viewer join snapshot` | 2995 | 100% | 100% | 40.9 ms |
| `S3 POST WS ticket` | 2995 | 100% | 100% | 19.1 ms |
| `S3 WS handshake complete` | 2995 | 100% | 100% | 903 ms |
| `S3 WS first snapshot/business message` | 2995 | 100% | 100% | 151 ms |
| `S3 live fanout receive` | 2995 | 100% | 100% | 52.2 ms |
| `S3 WS close` | 2995 | 100% | 100% | 129 ms |
| three reader APIs | 499 each | 100% | 100% | 16.2-53.5 ms |

PTS console exported tail latency:

| Sampler | Tail |
|---|---:|
| `S3 POST accepted-update bid` | p99 50.5 ms |
| `S3 viewer join snapshot` | p99 86.3 ms |
| `S3 POST WS ticket` | p99 55.7 ms |
| `S3 WS handshake complete` | p99 1.02 s |
| `S3 WS first snapshot/business message` | p99 310 ms |
| `S3 live fanout receive` | p99 124 ms |
| `S3 WS close` | p99 165 ms |
| `S3 GET auction snapshot` | p95 122 ms |
| `S3 GET auction leaderboard` | p99 181 ms |
| `S3 GET my bid history` | p99 110 ms |

Service-side confirmation:

| Signal | Value | Meaning |
|---|---:|---|
| Redis engine decisions | 998 | bidder rows reached the hot path |
| Settled decisions | 998/998 | no Redis/Kafka/PG settlement gap |
| Accepted/rejected | 7 / 991 | accepted publishes are the fanout source; rejects are normal final-burst decisions |
| Kafka lag | 0 | settlement consumer drained |
| Outbox pending | 0 | publication plane drained |
| `auction_ws_recover_total{snapshot_redis}` | 2995 | viewers received Redis hot snapshot on join |
| `auction_ws_publish_subscribers_sum` | 20965 | `7 accepted publishes * 2995 subscribers` |
| WS send queue depth | 0 | no observed queue buildup |
| WS connections after run | 0 | clean close after the PTS run |

Sampling caveat:

- The run used 1% sampling. Sampling logs are therefore diagnostic response-body
  evidence, not full coverage. The logs sampled 45 `S3 live fanout receive`
  rows; they all succeeded and carried `S3_V6_LIVE_FANOUT_OK...WS_ONLY`, with
  sampled viewers seeing 7 live messages.
- `S3 live fanout receive=2995` is a JMeter sampler count, not a raw WebSocket
  message count. Each viewer executes one receive/observe sampler; inside that
  sampler, the listener counts how many live events arrived. The raw fanout
  volume is confirmed service-side as `auction_ws_publish_subscribers_sum=20965`
  = `7 accepted publishes * 2995 subscribers`.
- `S3 live fanout receive p99=124ms` is the PTS p99 over those 2995 sampler
  results. It is not the full per-message p99 over 20965 WebSocket deliveries
  and not a p99 over per-viewer averages. The sampled response markers carry the
  per-viewer diagnostic value (`MAX_LAT_MS_xxx`) for the messages seen during
  that viewer's 30s observe window.
- Local `GetJMeterReportDetails` returned `AllCount=499` for long WebSocket
  receive/close samplers, but the PTS console API-list export reports `2995`.
  For judge-facing evidence, cite the console export and service-side
  `publish_subscribers_sum` together; keep the CLI mismatch as a tooling note.

Judge answer:

> "`20L8X79G` is not a pure fanout capacity curve; it is the mixed live-room
> integration rehearsal. It shows about 3000 same-room viewers joining through
> Redis snapshot recovery, 998 bid decisions settling with Kafka lag zero, and
> 7 accepted price publishes reaching 2995 subscribers. PTS console export shows
> the live receive sampler ran 2995 times with 100% request/assertion success and
> 52.2ms average response time. Because the run used 1% sampling, sampling logs
> are used only to inspect response markers, not to prove exact counts."

Industrial comparison answer:

> "For this scale, the fanout number is good. A 2995-viewer mixed room with
> `S3 live fanout receive` p99 124ms is below the common 200ms good
> responsiveness bar and far below the 1s live-room synchronization SLO. The bid
> p99 50.5ms and ticket p99 55.7ms are also strong. The only chart I would not
> oversell is handshake p99 1.02s: it is acceptable for a one-time WebSocket
> establishment under PTS, but it is not elite and should be explained separately
> from live fanout."

If asked exactly how the p99 was calculated:

> "The PTS p99 is across JMeter sampler results, one receive sampler per viewer,
> not across each WebSocket frame. The raw fanout count is 7 accepted publishes
> times 2995 subscribers, proven by service metrics. The sampling logs then show
> representative per-viewer markers, where sampled viewers received 7 live
> messages and reported low `MAX_LAT_MS` values."

## 8. Current Limitations And Next Evidence

Current S3 is local raw evidence. It is credible for internal explanation, but a
final judge pack should add:

1. PTS S3 cost variant: 2000 WS, same-VPC pressure IPs, PTS `ws-fanout-receive`
   p99 chart.
2. Local or PTS resource panel: active connections, RSS/connection,
   goroutines, open fds, CPU.
3. A completed 10k hold only after generator/source-port limits are controlled.

Until those exist, the final report should say "1000-WS local S3 pass; 2000+
single-node capacity still under diagnosis."
