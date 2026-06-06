# P1-E/P1-F Industrial Research And Risk Notes

Date: 2026-06-06

Scope: close the remaining gaps for P1-E server-side cinematic/highlight pipeline and P1-F WebSocket incremental leaderboard without adding latency or backpressure to the bid decision path.

## External References

- Redis leaderboard guidance: Redis Sorted Sets are the standard primitive for real-time ranking because `ZADD`, rank lookup, and top-N reads are cheap and do not require scanning bid rows on every viewer refresh. Reference: <https://redis.io/tutorials/howtos/leaderboard>
- Centrifugo real-time leaderboard pattern: the reference architecture combines Redis state, Redis Stream/event consumption, WebSocket publication, versioned payloads, recovery, and delta compression. Reference: <https://centrifugal.dev/blog/2025/04/28/websocket-real-time-leaderboard>
- AWS Media Replay Engine: highlight/replay creation is modeled as an event-driven clipping pipeline, not as synchronous live interaction work. Reference: <https://github.com/awslabs/aws-media-replay-engine>
- Amazon IVS timed metadata: live video systems attach timed metadata to the stream so overlays/highlights can be synchronized without coupling video delivery to auction transaction state. Reference: <https://docs.aws.amazon.com/ivs/latest/LowLatencyUserGuide/metadata.html>

## P1-F Leaderboard Architecture

Decision:

- Keep bid decision and settlement hot paths unchanged.
- Generate `leaderboard_delta` only after a durable auction event is published by the outbox relay.
- Treat leaderboard delta as a non-critical projection event. The existing WS queue/slow-consumer policy may drop or close slow clients; clients recover through snapshot/REST.
- Use latest-wins semantics with a monotonically increasing `seq`. H5 discards older deltas.
- Keep REST `/leaderboard` as recovery and manual refresh path.

Risk:

- If every accepted bid forces every H5 client to call REST, PG read pressure grows exactly when the last-second auction is hottest.
- If full rank recomputation is placed inside `PlaceBid` or Redis Lua, it competes with the p99 bid target.
- If every intermediate rank swap animates, the UI looks noisy under burst bidding. Client should coalesce visually through state replacement and bounded FLIP duration.

Mitigation:

- The relay computes compact top-N from accepted bid rows after publish. This is outside the bid response boundary.
- Payload is limited to top 5 plus buyer-safe heat fields.
- The client updates rank state directly from WS delta and keeps the refresh button as an explicit recovery action.

## P1-E Highlight Architecture

Decision:

- Add server-side highlight jobs/assets, requested by host API and safe to call after terminal events.
- Store deterministic HTML highlight reels as server assets for the internal competition. This closes the server pipeline gap without pretending to be a transcoding farm.
- Keep browser-generated WebM as immediate buyer-side export; server assets are host/operations artifacts and can later be replaced by FFmpeg/MRE-style rendering.

Risk:

- Video rendering/transcoding can be CPU-heavy and bursty immediately after auction end.
- If terminal event handling waits for video generation, payment/order handoff suffers.
- If highlight generation uses private bidder identities, shareable artifacts become unsafe.

Mitigation:

- Job creation and asset generation happen after the recap request and never block bid settlement, terminal events, order creation, or payment.
- Assets use item title, final price, masked winner, accepted bid count, bidder count, and extension count only.
- Job status and risk notes are persisted so future worker/transcoder replacement has a stable contract.

## Acceptance

- P1-F is complete only when H5 can update leaderboard from real WS `leaderboard_delta` without REST polling after each auction event.
- P1-E is complete only when the backend stores a server-side highlight job and asset that PC can surface, while H5 keeps its immediate downloadable highlight.
- Both items must document that the high-pressure auction hot path is not affected by cinematic rendering or rank projection.
