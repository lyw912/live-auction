# H5 Media Playback Contract

Phase 2 introduces a media-only descriptor API:

- `GET /api/live/sessions/{auction_id}`
- Response shape: `MediaPlayback`
- Frontend source priority: `ll-hls -> hls -> mp4 -> poster`

The descriptor is intentionally isolated from auction truth. It must not carry price, winner, status, seq, end time, rules, settlement, or payment state. Bidding, countdown, recovery, and payment continue to use the existing auction REST/WS/server-time contracts.

Phase 3 switches the same descriptor to MediaMTX LL-HLS by configuration:

```powershell
$env:LIVE_DEMO_MEDIA_PROTOCOL='ll-hls'
$env:LIVE_DEMO_MEDIA_URL='http://127.0.0.1:8888/auction-live/index.m3u8?cookieCheck=1'
$env:LIVE_DEMO_MIME_TYPE='application/vnd.apple.mpegurl'
$env:LIVE_DEMO_IS_LIVE='true'
$env:LIVE_DEMO_LATENCY_MS='3000'
$env:LIVE_MEDIA_FALLBACK_MP4_URL='/demo/jade-live-loop.mp4'
```

Run the stream smoke:

```powershell
.\scripts\smoke-mediamtx-llhls.ps1
```

The `cookieCheck=1` query is MediaMTX's direct playlist URL after its browser cookie probe redirect. It avoids descriptor clients receiving the HTML player page.

The H5 player consumes the same `MediaPlayback.sources` contract for static MP4, HLS, LL-HLS, and the disabled WHEP seam. WHEP stays `canPlay=false` in this phase.
