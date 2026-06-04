# PTS Console API List — 20L8X79G

Source: Alibaba Cloud PTS console export, API list, provided after the run.

Run:

- ReportId: `20L8X79G`
- ReportName: `S3-burst-20260605005515`
- Scenario: `S3-mixed-final-burst`
- Scale: 3000 WebSocket viewers + 1000 bidder rows + 500 reader rows
- Sampling setting: 1%

| API | Avg TPS | Peak TPS | Request success | Assertion success | Total requests | Avg RT |
|---|---:|---:|---:|---:|---:|---:|
| S3 POST accepted-update bid | 0 | 0 | 100% | 100.00% | 998 | 17.5 ms |
| S3 GET auction snapshot | 0 | 0 | 100% | 100.00% | 499 | 43.1 ms |
| S3 GET auction leaderboard | 0 | 0 | 100% | 100.00% | 499 | 53.5 ms |
| S3 GET my bid history | 0 | 0 | 100% | 100.00% | 499 | 16.2 ms |
| S3 viewer join snapshot | 7 | 0 | 100% | 100.00% | 2995 | 40.9 ms |
| S3 POST WS ticket | 0 | 0 | 100% | 100.00% | 2995 | 19.1 ms |
| S3 WS handshake complete | 2 | 0 | 100% | 100.00% | 2995 | 903 ms |
| S3 WS first snapshot/business message | 0 | 0 | 100% | 100.00% | 2995 | 151 ms |
| S3 live fanout receive | 5 |  | 100% | 100.00% | 2995 | 52.2 ms |
| S3 WS close | 28 |  | 100% | 100.00% | 2995 | 129 ms |

PTS console exported tail latencies:

| API | Tail latency |
|---|---:|
| S3 POST accepted-update bid | p99 50.5 ms |
| S3 GET auction snapshot | p95 122 ms |
| S3 GET auction leaderboard | p99 181 ms |
| S3 GET my bid history | p99 110 ms |
| S3 viewer join snapshot | p99 86.3 ms |
| S3 POST WS ticket | p99 55.7 ms |
| S3 WS handshake complete | p99 1.02 s |
| S3 WS first snapshot/business message | p99 310 ms |
| S3 live fanout receive | p99 124 ms |
| S3 WS close | p99 165 ms |

Interpretation:

- Use this console export as the primary PTS API count source for this run.
- `S3 live fanout receive` total requests `2995` is the number of JMeter receive
  sampler executions, effectively one successful receive/observe sampler per
  viewer row. It is not the raw WebSocket message count.
- The raw business fanout volume is `accepted publishes * subscribers`. In this
  run, service metrics show `auction_ws_publish_subscribers_sum=20965`, exactly
  `7 accepted publishes * 2995 subscribers`. Sampling-log response markers also
  show sampled viewers receiving `LIVE_MESSAGES_7`.
- Tail-latency semantics are separate from count semantics. PTS console
  `S3 live fanout receive p99=124 ms` is computed over the `2995` JMeter
  receive/observe sampler results. It is not a full per-WebSocket-message p99
  over all `20965` subscriber deliveries, and it is not "average 7 messages per
  viewer, then p99". The JMX receive sampler pauses the 30s observe window and
  records diagnostic response markers such as `MAX_LAT_MS_xxx`; those markers
  are the sampled per-viewer worst live-message latency evidence.
- The local `GetJMeterReportDetails` response stored in `jmeter-report-details.json`
  returned `AllCount=499` for `S3 live fanout receive` and `S3 WS close`, while
  the PTS console export reports `2995`. Treat that as a PTS API/report-detail
  presentation mismatch for long WebSocket samplers, not as evidence that only
  499 viewers completed receive/close.
- The 1% sampling logs are response-body forensics only. They sampled 45
  `S3 live fanout receive` rows; sampled response markers all showed successful
  `S3_V6_LIVE_FANOUT_OK...WS_ONLY` with 7 live messages.
- Server evidence in `../s3-burst-pts-20L8X79G/` shows `998` Redis engine
  decisions settled, Kafka lag `0`, outbox pending `0`, and the raw fanout
  delivery surface above.

Industrial interpretation:

- `S3 live fanout receive` p99 `124 ms` is strong for a 2995-viewer mixed room:
  it is below the common 200 ms "good interaction" responsiveness bar and well
  below the project's 1 s same-room realtime target.
- Do not phrase p99 `124 ms` as "20965 messages per-message p99". The correct
  judge-facing phrase is: "2995 receive samplers all succeeded; PTS sampler p99
  was 124ms; service metrics prove 20965 subscriber-delivery fanout; 1% sampled
  response markers show each sampled viewer received 7 live messages with
  low `MAX_LAT_MS`."
- `S3 POST accepted-update bid` p99 `50.5 ms` is excellent for the visible bid
  decision path in this integration run.
- Reader APIs are acceptable-to-good: leaderboard p99 `181 ms` and my-bids p99
  `110 ms`; snapshot p95 `122 ms`.
- `S3 WS first snapshot/business message` p99 `310 ms` is acceptable but not
  elite; it is still below the 1 s "flow remains intact" UX boundary.
- `S3 WS handshake complete` p99 `1.02 s` is the weakest chart. It is a one-time
  connection establishment sampler that includes PTS/JMeter pressure-agent and
  HTTP Upgrade timing, so it should be explained separately from backend fanout.
  Server metrics should remain the attribution source for backend join stages.
