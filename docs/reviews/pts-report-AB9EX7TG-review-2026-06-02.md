# PTS Report AB9EX7TG Review

> Date: 2026-06-02
> Classification: CURRENT_ADJACENT / HARNESS_GAP
> Workload: intended L2-P3 bid + WebSocket + reads combined first run
> Git SHA: 92fa6fd
> Runtime profile: `BID_ENGINE_MODE=redis_ledger`, `ADMISSION_ENABLED=false`

## Verdict

`AB9EX7TG` is strong L2-P3 diagnostic evidence, but it should not be promoted as
formal L2-P3 PASS evidence.

The system-side result is good: bid p99 stayed below the L2-P3 100ms ceiling,
read p99 stayed far below the 200ms ceiling, Redis/Kafka/PostgreSQL settlement
converged, and outbox drained. The run also proved active fanout during the bid
window.

The harness model drifted: the intended model was `1000 bid + 5000 WS + 1000
reader VU`, but server counters show `9987` WebSocket ticket/upgrade attempts
and the PTS sampling logs show only `987` bid samples. This is not a backend
loss: PostgreSQL also has exactly `987` unique bid rows, unique users, and
unique client bid ids. The missing 13 bids are on the PTS/JMeter execution side.

## Load Model Observed

Intended panel:

- Virtual user mode, manual traffic.
- Max VU: 7000.
- Duration: 1 minute.
- PTS loop cap: disabled.
- Specified IPs: 14.

Observed from PTS sampling logs:

| Sampler | Samples | Status | p50 | p95 | p99 | Max |
|---|---:|---|---:|---:|---:|---:|
| `POST L2-P3 hot bid` | 987 | 987 x 200 | 15ms | 50ms | 90ms | 108ms |
| `GET auction snapshot` | 20065 | 20065 x 200 | 1ms | 9ms | 22ms | 273ms |
| `GET auction leaderboard` | 20160 | 20160 x 200 | 2ms | 14ms | 34ms | 192ms |
| `GET my bid history` | 20089 | 20082 x 200, 7 socket closed | 1ms | 6ms | 21ms | 128ms |
| `open and hold WebSocket viewer` | 8606 | 8606 x 200 | 1059ms | 3658ms | 3901ms | 4051ms |

### What WS And Read Mean

`WS` means a viewer has opened the auction room realtime channel and is waiting
for push updates. In product terms, this is the user who is already inside the
live room and should see price changes without polling.

`read` means ordinary HTTP reads from users or clients that refresh/poll state:

- auction snapshot: current price, winner, state, timing;
- leaderboard: ranked visible auction/bid state;
- my bid history: the current user's own bid history.

They are different pressure types:

- WS stresses connection management, fanout, heartbeat, per-room subscriber
  lists, and per-message delivery delay.
- Reads stress request routing, auth/session lookup, database/cache read paths,
  and response serialization.

### Time Distribution

This run was not "all users hit at exactly the same millisecond".

Observed time distribution, relative to the first sample in the report:

- Bid burst: `987` POST requests started from `+23s` to `+26s`.
  - `+23s`: 71
  - `+24s`: 70
  - `+25s`: 71
  - `+26s`: 775
- Reads: ran continuously for about 60s. Each reader loop had snapshot,
  leaderboard, then my-bids. Because PTS/JMeter split users across 14 pressure
  instances, the per-second read starts appear as repeating waves rather than a
  flat line. The total was about `60k` GET requests in 60s, or about `1000`
  GET/s aggregated across the three read samplers in this actual run.
- WS: the intended JMX model was one 5000-viewer wave. Observed server counters
  show `9987` WS ticket/upgrade attempts, so PTS/JMeter executed the WS path
  closer to two waves:
  - first wave sampled around report `+0s`;
  - second wave sampled around `+25s` to `+29s`.

Future reviews must include this distribution table. A total count without time
shape is insufficient because 5000 users over 60s and 5000 users in 1s are very
different load models.

Sampler timing window:

- Reads ran for about 60s and produced about 60k GET samples.
- Bid burst landed from run +23.9s to +27.0s.
- WS sampled rows completed from run start through about +55s.

## Server Evidence

Evidence directory:

`docs/perf/pts/evidence/incoming/AB9EX7TG/`

PostgreSQL summary:

- `bids=987`
- `accepted=11`
- `rejected=976`
- `outbox PUBLISHED=11`
- pending outbox / settlement rows: `0`
- DB wait state after run: idle clients only, no lock backlog.

Redis/Kafka/engine verifier:

- Existing L1/L4B verifier fails only the expected-count gates because it expects
  exactly 1000 bid decisions.
- All structural correctness gates pass:
  - winner equals highest accepted bid;
  - accepted bid count matches PostgreSQL;
  - engine sequence is complete from 1 through 987;
  - no duplicate client bid id;
  - no duplicate engine sequence;
  - Kafka offset order preserves engine order;
  - Kafka consumer lag drains to zero;
  - Redis pending decisions empty;
  - DLQ empty;
  - outbox drained.

Server metrics:

- `auction_bid_redis_ledger_total{result="ENGINE_ACCEPTED"} 11`
- `auction_bid_redis_ledger_total{result="ENGINE_REJECTED"} 976`
- `auction_bid_redis_settlement_total{result="ENGINE_ACCEPTED",status="settled"} 11`
- `auction_bid_redis_settlement_total{result="ENGINE_REJECTED",status="settled"} 976`
- `auction_admission_enabled 0`
- `http_request_total{method="GET",path="/ws",status="101"} 9987`
- `http_request_total{method="POST",path="/api/auth/ws-ticket",status="200"} 9987`
- `auction_ws_publish_subscribers_sum 54920`
- `auction_ws_publish_subscribers_count 11`
- `auction_fanout_latency_seconds_count 11`

The average subscribers per accepted price-change publication is about
`54920 / 11 = 4993`, matching the intended active 5000-viewer fanout window.

Server fanout latency histogram:

- `auction_fanout_latency_seconds_bucket{le="0.025"} 10`
- `auction_fanout_latency_seconds_bucket{le="0.05"} 11`
- `auction_fanout_latency_seconds_count 11`

This means all 11 accepted price-change publications completed fanout within the
50ms histogram bucket, and 10/11 completed within 25ms. Because only 11 accepted
price changes occurred, the empirical p99 is effectively the worst accepted
publication in this run and should be reported as `<=50ms bucket`, not as a
precise millisecond value.

## Interpretation

Do not use the all-sampler PTS p99 for this run. The all-sampler p99 is dominated
by the WebSocket JSR223 sampler and does not represent bid latency. The correct
interpretation must be per sampler:

- Bid user-visible HTTP p99: `90ms`.
- Server-core bid total histogram: all 987 decisions are within the 25ms bucket.
- Read p99: `21ms` to `34ms` depending on endpoint.
- Fanout p99 from server histogram: within the 50ms bucket for the 11 accepted
  publications.
- WS sampler p99 around 3.9s measures Java load-generator open/ticket/connect
  behavior plus paused sampler edge effects, not bid decision latency.

The WS sampler p99 is not "bid response time" and is not "price update delay".
It is the measured active portion of the JSR223 WebSocket sampler after excluding
most of the socket hold time with `SampleResult.samplePause()`. In this run the
WS sampler also includes:

- POST `/api/auth/ws-ticket`;
- Java client WebSocket upgrade/open;
- listener setup and close path;
- residual timing around the pause/resume boundary.

The same rows had about 25s of `idleTime`, so the wall-clock user session was
roughly 26s to 29s, while the reported sampler elapsed p99 was about 3.9s.

Do not interpret this as "a real viewer waits 3.9s to enter the live room"
without a dedicated join-latency test. A real user join test should separately
measure:

1. HTTP room snapshot load;
2. WS ticket issue latency;
3. WebSocket upgrade latency;
4. time to first snapshot/history/realtime message.

For a production UX target, this project should treat join p95/p99 in seconds as
too slow unless there is a documented mobile-network reason. `AB9EX7TG` only
shows that the current Java/Pts WebSocket harness has multi-second sampler
overhead under thousands of connections; it does not prove real client join UX.

This run is therefore a useful capacity signal: under roughly 987 bid decisions,
about 60k read GETs, and active fanout to about 5000 subscribers during accepted
price changes, the backend did not saturate.

## Fanout Coverage And Limits

What this run proves:

- accepted price updates were published while about 5000 subscribers were active;
- server-side fanout work for the 11 accepted publications completed within the
  50ms histogram bucket;
- every sampled WS client reported receiving `WS_MESSAGES_11` or `WS_MESSAGES_12`.

What this run does not fully prove:

- it does not prove that every one of the approximately 5000 connected viewers
  received every update before disconnect;
- it does not prove long-hold heartbeat stability;
- it does not prove reconnect-storm recovery after network loss.

To prove "all connected viewers see timely updates" rigorously, the WS workload
needs per-client sequence validation: each WS client should track the highest
auction seq received, emit it as a metric/sample field, and the verifier should
check that connected clients observed seq `11` within the fanout SLA. Server
histograms are necessary but not sufficient because they prove publish-side
completion, not client-side receipt for every connection.

## Why It Is Not Formal PASS

The formal L2-P3 workload definition must be deterministic and defensible:

- bid count should be exactly 1000, or the runbook must explicitly allow a
  tolerance and explain why;
- WS connection attempts should match the configured `ws_threads=5000`, not
  roughly 10000 server-side attempts;
- the verifier used for L2-P3 should check the observed bid-count target or an
  L2-P3-specific tolerance instead of failing L1/L4B exact-1000 gates.

Because these harness conditions are not yet clean, this report is
`CURRENT_ADJACENT`, not `CURRENT_PASS`.

## Likely Root Cause

`是否指定循环=否` was correct for the duration-driven read ThreadGroup, but it
appears to let one-loop WS/bid groups interact with PTS/JMeter execution in a way
that is not identical to the intended 5000 WS / 1000 bid model.

Observed split:

- Bid samples per PTS instance: 70 to 71, totaling 987 across 14 instances.
- WS sampled rows per PTS instance vary from 356 to 714, totaling 8606 samples.
- Server-side WS ticket/upgrade counters total 9987, which implies the WS path
  was attempted almost twice relative to the intended 5000.

This points to harness scheduling / PTS loop semantics, not to backend overload.

## Next Action

Before rerunning L2-P3 as a formal pass:

1. Split the scenario so one-shot bid/WS thread groups are protected from PTS
   external looping while read traffic remains duration-driven.
2. Prefer one of these approaches:
   - keep L2-P3 as a single JMX but make bid/WS idempotent one-shot using a
     JMeter property or per-user guard, then allow reads to loop by duration; or
   - run a formal combined test with `是否指定循环=是, 循环次数=1` and move read
     repetition fully into an inner controller whose loop count is deterministic.
3. Add an L2-P3 verifier that accepts an explicit expected bid count and checks
   the L2-specific gates separately from L1-C1 exact-1000 rules.
4. Rerun only after reset and fresh preflight.

This report is not CURRENT_PASS evidence for current L2-P3 because the harness
load shape drifted from the intended 1000 bid / 5000 WS model and exact bid-count
gates failed.
