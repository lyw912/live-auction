# PTS Report 3IVNW7TF Analysis Summary

Date: 2026-05-28

This summary records the sanitized findings from Alibaba Cloud PTS report
`3IVNW7TF`. Raw PTS sampling logs are intentionally not committed because they
contain live bearer tokens, request headers, and temporary WebSocket tickets.

## PTS Overview

```text
ReportId: 3IVNW7TF
ReportName: pts-20260528013714
StartTime: 2026-05-28 01:37:14
EndTime:   2026-05-28 01:47:14
AgentCount: 1
Vum: 4967
AllCount: 383580
AvgTps: 643.44
AvgRt: 58.13 ms
Seg90Rt: 110 ms
Seg99Rt: 152.99 ms
FailCountReq: 0
SuccessRateReq: 100%
```

## Sampler Metrics

| Sampler | Count | Avg TPS | Avg RT | P90 | P99 | Max | Fail |
|---|---:|---:|---:|---:|---:|---:|---:|
| `POST bid downstream pressure` | 313905 | 526.57 | 62.91 ms | 111 ms | 152.98 ms | 456 ms | 0 |
| `POST ws-ticket issue` | 45658 | 76.59 | 35.73 ms | 55 ms | 62 ms | 284 ms | 0 |
| `GET snapshot under bid pressure` | 13134 | 22.03 | 47.26 ms | 72 ms | 80 ms | 237 ms | 0 |
| `GET auction snapshot auth ACL` | 3627 | 6.08 | 52.32 ms | 71 ms | 79.72 ms | 157 ms | 0 |
| `GET /readyz` | 3628 | 6.09 | 14.54 ms | 21 ms | 26 ms | 153 ms | 0 |
| `GET /metrics admission flag` | 3628 | 6.09 | 14.54 ms | 20 ms | 26 ms | 56 ms | 0 |

## Sampled Business Distribution

`GetJMeterSamplingLogs` returned 3895 sampled entries, not the full 383580
request set. In the sampled bid responses:

```text
REJECTED / BID_TOO_LOW:   1948
REJECTED / AUCTION_ENDED: 1064
ACCEPTED:                 176
Unparsed/non-bid body:     10
```

Interpretation: PTS HTTP success rate is not the same as accepted bid capacity.
Most sampled bid requests were business rejections.

## Backend Metrics

Admission was disabled:

```text
auction_admission_enabled 0
```

Bid request counters:

```text
AUCTION_ENDED rejected: 104646
BID_TOO_LOW rejected:  193555
ACCEPTED:               18196
ACCEPTED_EXTENDED:          3
```

Bid lock and latency:

```text
auction_bid_lock_wait_seconds_count 316400
auction_bid_lock_wait_seconds_sum   2593.008
avg lock wait ~= 8.2 ms

auction_bid_latency_seconds_count 316400
auction_bid_latency_seconds_sum   8700.134
avg backend bid latency ~= 27.5 ms
```

Redis publish pipeline:

```text
redis_command_latency_seconds_count{command="outbox_publish_pipeline"} 8591
redis_command_latency_seconds_sum{command="outbox_publish_pipeline"}   2.485 s
avg ~= 0.29 ms
```

## Database Findings

```text
bids total: 316400
accepted:    18199
rejected:   298201

outbox event bid_rejected: 298201
outbox event bid_accepted:  18199
```

Outbox delivery backlog observed after the run:

```text
PENDING:   >304000
PUBLISHED: ~11695
pending shard: 13
oldest pending age: >1 hour
```

## Verdict

The run found a real bottleneck in the app-owned outbox relay and realtime
delivery chain. It did not prove accepted-bid capacity or WebSocket fanout
capacity.

Primary conclusion:

```text
HTTP layer remained stable at the measured load, but durable realtime delivery
could not keep up with event production. Outbox relay must be redesigned before
claiming production-grade realtime performance.
```
