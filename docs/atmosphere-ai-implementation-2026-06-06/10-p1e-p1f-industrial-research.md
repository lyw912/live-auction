# P1-E/P1-F Industrial Research And Risk Notes

Date: 2026-06-06

Scope: close the remaining gaps for P1-E server-side cinematic/highlight pipeline and P1-F WebSocket incremental leaderboard without adding latency or backpressure to the bid decision path.

## External References

- Redis leaderboard guidance: Redis Sorted Sets are the standard primitive for real-time ranking because `ZADD`, rank lookup, and top-N reads are cheap and do not require scanning bid rows on every viewer refresh. Reference: <https://redis.io/tutorials/howtos/leaderboard>
- Centrifugo real-time leaderboard pattern: the reference architecture combines Redis state, Redis Stream/event consumption, WebSocket publication, versioned payloads, recovery, and delta compression. Reference: <https://centrifugal.dev/blog/2025/04/28/websocket-real-time-leaderboard>
- AWS Media Replay Engine: highlight/replay creation is modeled as an event-driven clipping pipeline, not as synchronous live interaction work. Reference: <https://github.com/awslabs/aws-media-replay-engine>
- Amazon IVS timed metadata: live video systems attach timed metadata to the stream so overlays/highlights can be synchronized without coupling video delivery to auction transaction state. Reference: <https://docs.aws.amazon.com/ivs/latest/LowLatencyUserGuide/metadata.html>
- MDN `requestAnimationFrame`: animation updates should be scheduled before the next repaint; H5 uses this to coalesce burst rank deltas into one visible frame. Reference: <https://developer.mozilla.org/en-US/docs/Web/API/Window/requestAnimationFrame>

## P1-F Leaderboard Architecture

Decision:

- Keep bid decision and settlement hot paths unchanged.
- Generate `leaderboard_delta` only after a durable auction event is published by the outbox relay.
- Treat leaderboard delta as a non-critical projection event. The outbox relay now only enqueues a lightweight projection task into a bounded in-process queue (`LEADERBOARD_PROJECTION_QUEUE_SIZE`, default `1024`; `LEADERBOARD_PROJECTION_WORKERS`, default `1`). If the queue is full, the projection is skipped rather than blocking original auction event fanout.
- The existing WS queue/slow-consumer policy may drop or close slow clients; clients recover through snapshot/REST.
- Use latest-wins semantics with a monotonically increasing `seq`. H5 discards older deltas.
- H5 applies burst deltas through `requestAnimationFrame`, keeps only the latest pending rank state, suppresses repeated rank-change sound during burst mode, and shortens/skips noisy FLIP animation under sub-180ms updates. REST leaderboard responses are also seq-guarded so a slow initial/read refresh cannot overwrite a newer WS delta.
- Keep REST `/leaderboard` as recovery and manual refresh path.

Risk:

- If every accepted bid forces every H5 client to call REST, PG read pressure grows exactly when the last-second auction is hottest.
- If full rank recomputation is placed inside `PlaceBid` or Redis Lua, it competes with the p99 bid target.
- If every intermediate rank swap animates, the UI looks noisy under burst bidding. Client should coalesce visually through state replacement and bounded FLIP duration.

Mitigation:

- The relay does not compute compact top-N inline. It enqueues projection work after publishing the original auction event; worker-side build has an 80ms context timeout and metrics for enqueue result, build latency, and queue lag.
- Payload is limited to top 5 plus buyer-safe heat fields.
- The client updates rank state directly from WS delta and keeps the refresh button as an explicit recovery action.

## P1-E Highlight Architecture

Decision:

- Add server-side highlight jobs/assets, requested by host API and safe to call after terminal events.
- Store server-rendered WebM highlight reels as recap assets for the internal competition. This closes the server pipeline gap without pretending to be a distributed transcoding farm.
- Keep browser-generated WebM as immediate buyer-side export; server assets are host/operations artifacts and can later be replaced by MRE-style rendering.

Risk:

- Video rendering/transcoding can be CPU-heavy and bursty immediately after auction end.
- If terminal event handling waits for video generation, payment/order handoff suffers.
- If highlight generation uses private bidder identities, shareable artifacts become unsafe.

Mitigation:

- Job creation and asset generation happen after the recap request and never block bid settlement, terminal events, order creation, or payment.
- Assets use item title, final price, masked winner, accepted bid count, bidder count, and extension count only.
- FFmpeg rendering is limited to one concurrent render per process and reuses a recent rendered WebM asset for the same auction within 10 minutes. Repeated host clicks therefore return an existing artifact instead of starting repeated CPU-heavy renders.
- Job status and risk notes are persisted so future worker/transcoder replacement has a stable contract.

## Isolation Addendum

Implemented on 2026-06-06 after the pressure review:

- `PlaceBid`, Redis Lua decisioning, Kafka ACK durability, terminal winner selection, order creation, and payment handoff were not changed.
- `leaderboard_delta` is asynchronous, bounded, observable, and droppable. Dropped deltas are acceptable because the next delta or manual REST refresh recovers the display.
- H5 rank display is latest-wins, frame-coalesced, and protected against stale REST overwrite, so final-second burst bidding should feel like fast momentum rather than three separate jittery row swaps or a backwards jump.
- AI commentary worker now has configurable batch size and per-task timeout (`AI_COMMENTARY_BATCH_SIZE`, `AI_COMMENTARY_TASK_TIMEOUT`). Provider slowness records a failed/retryable job and does not block bid acknowledgement.
- Server WebM highlight rendering has per-process concurrency limit and recent-asset reuse. It remains a host recap action, not terminal settlement work.
- Slow WS clients remain protected by bounded message/byte queues and policy-close behavior.

## Acceptance

- P1-F is complete only when H5 can update leaderboard from real WS `leaderboard_delta` without REST polling after each auction event.
- P1-E is complete only when the backend stores a server-side highlight job and asset that PC can surface, while H5 keeps its immediate downloadable highlight.
- Both items must document that the high-pressure auction hot path is not affected by cinematic rendering or rank projection.
