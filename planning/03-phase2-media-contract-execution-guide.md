# Phase 2 统一媒体播放契约 —— 工程落地执行手册

> **本文性质**：给「实现模型 / 实现者」直接照着做的**执行手册（playbook）**。论证与选型理由见 `planning/01-frontend-media-payment-refactor.md`（下称「调研稿」§3.1、§5.1、§5.2、§7-Phase2）。本文只讲**做什么、按什么顺序、做到什么算完、绝不能碰什么**。
> **前置**：本文假设 `planning/02-phase1-rebuild-execution-guide.md`（Phase 1）已完成——尤其是它留下的**媒体薄口子** `features/live-media/useLiveMediaSource`（返回 `LiveMediaSourceV0{kind:'video-file'}`）与 `widgets/live-stage/LiveBackdrop`。Phase 2 就是把这个 V0 口子**升级成正式契约 + 适配器系统 + 后端描述符接口**。
> **本期范围**：调研稿 **Phase 2：统一媒体播放契约**全部——① 定型 `MediaPlayback` 契约；② 新增后端 `GET /api/live/sessions/{id}` 返回描述符（demo 仍返回 MP4 描述符）；③ 前端媒体源适配器系统（`mp4` + `hls/ll-hls` 适配器，`whep` 仅留接缝）；④ 降级链 `ll-hls → hls → mp4/海报`；⑤ **播放与竞拍实时彻底解耦**的验证。
> **本期不做（留给 Phase 3 及以后）**：不部署 MediaMTX、不产真实直播流（Phase 3）；不实现 WebRTC/WHEP（仅留适配器接缝，Future）；不做多码率/CDN/回源鉴权（Future）；不引入摄像头/采集端。Phase 2 用**本地静态 LL-HLS 样例文件**验证 hls.js/Safari 两条路径，不依赖任何流媒体服务器。详见 [§1.2](#12-不做留口子) / [§9](#9-本地-ll-hls-样例验证不依赖-mediamtx) / [§12](#12-留给-phase-3-及以后的口子)。
> **最高红线**：**媒体与竞拍真相彻底解耦**——Live Session API 绝不承载价格/赢家/终态；媒体掉线绝不影响出价/决策/恢复。且 Phase 1 的 I1–I7 不变量仍须全绿。见 [§2](#2-红线本期的头号红线是解耦)。

---

## 0. 给实现模型的元说明（先读这节）

### 0.1 工作方式（强制）

1. **演进，不是新建**：Phase 1 已把取流点收敛成 `useLiveMediaSource`/`<LiveBackdrop>`。Phase 2 是**原地把它们的内部实现升级**为契约 + 适配器，**调用方（`LiveStage`）尽量不动**。这是「换实现不换调用方」的兑现。
2. **后端最小侵入**：本期后端只**新增一个只读接口**（`GET /api/live/sessions/{id}`）和必要 config 字段；**不改** `bid.go`、不改 WS 协议、不改任何竞拍/结算/恢复代码、不加数据库列。
3. **解耦优先于美观**：本期的核心交付不是"播放更流畅"，而是**证明媒体子系统是一个可独立失败的隔离单元**。先把解耦做对、写成测试，再谈适配器细节。
4. **不提前造 Phase 3/WebRTC**：契约里 `protocol` 留 `'whep'` 枚举值和适配器接缝即可，**不写** WHEP 实现；不写 MediaMTX 配置。
5. **不确定就停**：任何改动若可能触达出价/恢复/WS，停下标 `// RED-LINE-REVIEW`，不要赌。

### 0.2 阅读顺序

`§2 红线` → `§1 范围` → `§3 结构` → `§4 契约定型` → `§5 后端接口` → `§6 适配器系统` → `§7 接线 LiveStage` → `§8 解耦验证`（本期重心）→ `§9 本地样例验证` → `§10 WP 顺序` → `§11 验收`。

### 0.3 完成的定义（全局 DoD）

一个 WP 算完成，当且仅当：① 该 WP 验收项全绿；② Phase 1 的 I1–I7 characterization test **仍全绿**；③ `LiveStage`/`LiveBackdrop` 内**不再出现任何写死的流地址或写死的协议分支**（全部经 `MediaPlayback` + 适配器）；④ **媒体掉线/接口 404/流损坏时，出价、确认、恢复、倒计时、氛围门控均无回归**（[§8](#8-解耦验证本期重心) 的隔离测试通过）；⑤ 后端未改竞拍/结算/恢复代码、未加 DB 列；⑥ 未引入 WebRTC/MediaMTX。

---

## 1. 本期范围与边界

### 1.1 做（In Scope）

| 块 | 内容 | 章节 |
|---|---|---|
| 契约定型 | 把 Phase 1 的 `LiveMediaSourceV0` 升级为正式 `MediaPlayback`（`MediaProtocol`/`MediaSource[]`/`capabilities`/降级链） | [§4](#4-mediaplayback-契约定型) |
| 后端描述符接口 | 新增 `GET /api/live/sessions/{id}`，返回 `MediaPlayback`；demo 由 **config 驱动**返回 MP4 描述符；只读、无竞拍真相 | [§5](#5-后端live-session-api) |
| 适配器系统 | `MediaAdapter` 接口 + `mp4` 适配器 + `hls/ll-hls` 适配器（Safari 原生 / hls.js 两路）；`whep` 仅接缝 | [§6](#6-前端媒体源适配器系统) |
| 接线 | `useLiveMediaSource` 改为经 TanStack Query 拉接口；`<LiveBackdrop>` 按 `protocol` 分发到适配器 + 降级链 | [§7](#7-接线-livestage--livebackdrop) |
| 解耦验证 | 媒体子系统作为可独立失败的隔离单元；media 故障不波及竞拍 | [§8](#8-解耦验证本期重心) |
| 本地验证 | 用 ffmpeg 离线产一份静态 LL-HLS 样例，验证 hls.js + Safari 原生两条路径 | [§9](#9-本地-ll-hls-样例验证不依赖-mediamtx) |

### 1.2 不做（留口子）

- **不部署 MediaMTX、不产真实直播流**（Phase 3）。Phase 2 的 demo 描述符仍指向**循环 MP4 文件**（与 Phase 1 同一段视频），只是改为经接口下发。
- **不实现 WebRTC/WHEP**：契约保留 `protocol:'whep'` 枚举与 `whepAdapter` 空接缝（throw "not implemented in Phase 2"），**不写实现**。
- **不做**多码率/自适应码率/CDN/回源鉴权/防盗链 token（Future）。
- **不改**后端竞拍/结算/恢复、不改 WS 协议、不加 DB 列、不动 `displayMediaURL`（封面改写照旧）。

### 1.3 与 Phase 1 / Phase 3 的接力关系

```
Phase 1: useLiveMediaSource(): LiveMediaSourceV0{kind:'video-file', url:demoMP4}   // 薄口子，直接放视频
   │  （本期升级，调用方不变）
   ▼
Phase 2: useLiveMediaSource(): MediaPlayback{sources:[…], capabilities, isLive}     // 经 API + 适配器 + 降级链
         后端 GET /api/live/sessions/{id} → 由 config 返回 MP4 描述符（demo）        // 仍是循环 MP4
   │  （Phase 3 只改 config + 接 MediaMTX，契约/前端不变）
   ▼
Phase 3: 同一接口返回 LL-HLS 描述符（MediaMTX 真流）；前端零改动即切到真直播
```

> **关键**：Phase 2 把"协议无关的契约 + 能跑 hls.js 的适配器"全部建好并用**静态 LL-HLS 样例**验证通过。这样 Phase 3 只需让后端返回真实 m3u8 地址，前端**一行不改**即从"循环 MP4"切到"真直播流"。

---

## 2. 红线：本期的头号红线是「解耦」

### 2.1 媒体 ⊥ 竞拍真相（最高优先级）

| # | 不变量 | 验证方式 |
|---|---|---|
| M1 | **Live Session API 不承载任何竞拍真相** | 接口响应体只含媒体字段（见 §4）；**禁止**出现 price/winner/status/seq/终态/结算字段。code review + 响应 schema 断言。 |
| M2 | **媒体子系统可独立失败** | 媒体接口 404 / 流地址不可达 / m3u8 损坏 / 解码失败时，竞拍页其余部分（价格、倒计时、出价、恢复、氛围）**完全正常**。[§8](#8-解耦验证本期重心) 隔离测试。 |
| M3 | **媒体不参与竞拍状态/时钟** | 倒计时仍由 `deriveCountdown` 服务端时间锚定（I4），**不得**用视频 `currentTime`/缓冲状态推断竞拍进度或落槌。 |
| M4 | **媒体查询与竞拍 WS 各自独立** | 媒体描述符走独立 TanStack Query（普通 HTTP 缓存），**不**复用竞拍 WS 连接、**不**共享错误边界、**不**互相阻塞渲染。 |

### 2.2 Phase 1 不变量仍须全绿（不得回归）

Phase 2 不应触碰 I1–I7，但解耦改动可能间接影响渲染树，所以每个 WP 仍要跑 Phase 1 的 I1–I7 characterization test：① 出价响应解读以服务端字段为准（非 HTTP 200）；② ws-ticket 接入；③ 恢复来源完整；④ 倒计时服务端锚定；⑤ 危险操作禁用；⑥ 出价幂等三层；⑦ 支付服务端真相 + PC 只读。详见 Phase 1 手册 §2。

### 2.3 强制规则

- 触碰竞拍页渲染树的 PR，必须在描述里点名「如何验证 M1–M4 与 I1–I7 未回归」。
- **媒体的任何失败态都不能 throw 进竞拍子树**：媒体 `features/live-media` 必须自带 error boundary / 自带 `status` 字段消化错误，对外只暴露"能播则播、不能播则降级/留海报"，绝不让媒体异常冒泡。

---

## 3. 目标工程结构

媒体相关代码集中在一处，便于"作为隔离单元"管理（沿用 Phase 1 的 FSD 分层与依赖规则）：

```
shared/media/
  contract.ts          # MediaPlayback / MediaProtocol / MediaSource 类型（前后端共享形状的唯一真相）
  detect.ts            # 浏览器能力探测：canPlayType / Hls.isSupported（纯函数）
entities/live-session/
  api.ts               # fetchLiveSession(auctionId): GET /api/live/sessions/{id}
  model.ts             # 把后端响应规整为 MediaPlayback（含降级链排序）
features/live-media/
  useLiveMediaSource.ts # Phase 1 同名 hook，本期内部改为：useQuery(fetchLiveSession) → MediaPlayback
  usePlaybackEngine.ts  # 适配器选择 + 降级链推进 + 播放状态（普通 hook，非 XState）
  adapters/
    types.ts           # MediaAdapter 接口
    mp4.ts             # 原生 <video src> 适配器
    hls.ts             # Safari 原生 HLS / hls.js（MSE）适配器，覆盖 'hls' 与 'll-hls'
    whep.ts            # 仅接缝：throw 'WHEP not implemented in Phase 2'
widgets/live-stage/
  LiveBackdrop.tsx     # Phase 1 同名组件，本期改为消费 usePlaybackEngine 的输出
  LiveStage.tsx        # 调用方，尽量不动（仍 const media = useLiveMediaSource(auctionId)）

backend/internal/gateway/
  auction_handlers.go  # 新增 AuctionHandler.LiveSession 方法（只读描述符）
  router.go            # 新增一行路由注册
backend/internal/config/
  config.go            # 新增 demo 媒体描述符的 config 字段（见 §5.3）
```

依赖规则不变：`widgets → features → entities → shared`，只能向下。媒体子树**不依赖**竞拍出价/连接的任何模块。

---

## 4. MediaPlayback 契约定型

> 这是前后端共享的"形状真相"。前端 `shared/media/contract.ts` 定义 TS 类型；后端按相同 JSON 形状返回（Go struct tag 对齐）。

### 4.1 类型（前端 `shared/media/contract.ts`）

```ts
export type MediaProtocol = 'mp4' | 'hls' | 'll-hls' | 'whep';
//                                                       ^ 仅枚举占位，本期不实现

export interface MediaSource {
  protocol: MediaProtocol;
  url: string;                 // demo: /demo/jade-live-loop.mp4 或本地样例 /demo/sample-llhls/index.m3u8
  mimeType?: string;           // 如 'application/vnd.apple.mpegurl'
  priority: number;            // 数值越小优先级越高；适配器按此 + 浏览器能力选择
}

export interface MediaPlayback {
  auctionId: string;
  isLive: boolean;             // demo MP4 = false（循环占位）；Phase 3 真直播 = true
  posterURL?: string;          // 封面（与流解耦；经 displayMediaURL 处理，可空）
  sources: MediaSource[];      // 按 priority 升序的来源 = 降级链
  latencyTargetMs?: number;    // ll-hls≈3000；仅用于 UI 文案/监控，不参与竞拍
  capabilities: {              // 服务端声明的可用能力（前端再用 detect.ts 二次确认）
    nativeHlsOnSafari: boolean;
    mseHls: boolean;
    webrtc: boolean;           // 本期恒 false
  };
  // 预留（本期可不填，Phase 3 用）：
  sessionEpoch?: string;       // Phase 3 流重启后用于 cache-busting / 重新拉描述符
}
```

> **schema 守卫（对应 M1）**：契约里**没有也禁止**出现 `priceCents`/`winnerId`/`status`/`seq`/`endAt` 等竞拍字段。加一个单测断言响应键集合 ⊆ 允许集合，防止有人日后往里塞竞拍真相。

### 4.2 降级链表达

降级链 = `sources` 的 priority 顺序。demo（Phase 2）典型返回：

```jsonc
// GET /api/live/sessions/{id} 的 demo 响应（Phase 2）
{
  "auctionId": "auc_123",
  "isLive": false,
  "posterURL": "/api/media/items/xxx.jpg",   // 或 null
  "sources": [
    { "protocol": "mp4", "url": "/demo/jade-live-loop.mp4", "mimeType": "video/mp4", "priority": 10 }
  ],
  "latencyTargetMs": null,
  "capabilities": { "nativeHlsOnSafari": true, "mseHls": true, "webrtc": false }
}
```

Phase 3 同接口将返回（前端无需改动）：

```jsonc
{
  "auctionId": "auc_123", "isLive": true, "posterURL": "/api/media/items/xxx.jpg",
  "sources": [
    { "protocol": "ll-hls", "url": "https://…/auc_123/index.m3u8", "mimeType": "application/vnd.apple.mpegurl", "priority": 10 },
    { "protocol": "hls",    "url": "https://…/auc_123/fallback.m3u8", "mimeType": "application/vnd.apple.mpegurl", "priority": 20 },
    { "protocol": "mp4",    "url": "/demo/jade-live-loop.mp4", "priority": 90 }
  ],
  "latencyTargetMs": 3000,
  "capabilities": { "nativeHlsOnSafari": true, "mseHls": true, "webrtc": false }
}
```

---

## 5. 后端 Live Session API

### 5.1 路由注册（`gateway/router.go`）

后端是 **chi**，路由集中在 `router.go`。auction 读接口（如 `GetAuction`）在**认证组**内（`r.Group` + `authMiddleware`），媒体代理 `ServeMedia` 在认证组**外**（`router.go:129`）。

**推荐：把 Live Session 放进认证组**（与 `GetAuction` 同级），路径 `{id}` 与现有 auction 路由保持一致：

```go
// 在 r.Group(func(r chi.Router){ r.Use(authMiddleware(...)) ...}) 内，GetAuction 附近加：
r.Get("/live/sessions/{id}", auctionHandler.LiveSession)
```

> 为什么认证组：① 前端访问竞拍页本就持有 auth 上下文，零额外成本；② 便于 Phase 3 做防盗链/scope；③ 描述符不含竞拍真相，但仍是"这个用户能看哪场"的信息。
> **替代选项**（需团队确认，列为 [§11.2](#112-开放问题) 开放问题）：若 demo 要免登录直链播放，可放到认证组外（与 `ServeMedia` 同位）。本期默认认证组。

### 5.2 Handler（`gateway/auction_handlers.go`，新增 `AuctionHandler.LiveSession`）

形态参照同文件既有 handler（`GetAuction`、`ServeMedia`）：用 `chi.URLParam(r, "id")` 取 auctionID，`writeResult`/`writeError` 统一返回。

```go
// 新增方法；只读；只描述媒体；不碰竞拍真相
func (h AuctionHandler) LiveSession(w http.ResponseWriter, r *http.Request) {
    auctionID := chi.URLParam(r, "id")
    if auctionID == "" {
        writeError(w, r, apierrors.New(apierrors.CodeInvalidArgument, "missing auction id", http.StatusBadRequest))
        return
    }
    // 可选：校验 auction 存在（用既有 Repo.GetAuction），不存在则 404。
    //   注意：即便读了 auction，也【只】用它取 item 的封面 ImageURL 作 posterURL，
    //   【绝不】把 price/winner/status 放进响应（M1）。
    playback := h.buildDemoPlayback(r.Context(), auctionID) // 见 §5.3：由 config 驱动
    writeResult(w, r, http.StatusOK, playback, nil)
}
```

> **重要事实（已核验）**：`auction.Item` 只有 `ImageURL`，**没有 `video_url` 列**（`model.go:19`）。Phase 1 里 H5 的 `video_poster_url` 是前端 demo 数据，不是 DB 字段。因此 **Phase 2 的流地址不来自 DB，而来自 config**（[§5.3](#53-config-驱动不写死)）。posterURL 可来自 item.ImageURL（经 `/api/media` 形式），可空。

### 5.3 config 驱动，不写死

不要把 `/demo/jade-live-loop.mp4` 硬编码进 handler。在 `config.Config` 加字段（沿用现有 env 读取惯例），让 Phase 3 只改配置即切换：

```go
// config.Config 新增（命名随项目惯例）：
LiveDemoMediaProtocol string // "mp4"（Phase 2 demo） / "ll-hls"（Phase 3）
LiveDemoMediaURL      string // Phase 2: "/demo/jade-live-loop.mp4"; Phase 3: MediaMTX m3u8
LiveDemoMimeType      string // 可空
LiveDemoIsLive        bool   // Phase 2: false; Phase 3: true
LiveDemoLatencyMs     int    // Phase 2: 0; Phase 3: 3000
```

`buildDemoPlayback` 用这些字段拼 `MediaPlayback`。**这样 Phase 3 不改一行 Go 逻辑，只改 env/config 就把 demo 描述符从 MP4 切到 LL-HLS。**

### 5.4 后端红线

- handler **只读**，无副作用，不写库、不发事件、不碰 Redis/Kafka。
- 响应**只含** `MediaPlayback` 字段（M1）；加一个 handler 级单测断言响应 JSON 不含竞拍键。
- 不改 `bid.go`/WS/迁移；不加 DB 列。

---

## 6. 前端媒体源适配器系统

> 目标：`<LiveBackdrop>` 不认协议、不认 hls.js，只把 `MediaPlayback.sources` 交给"选择器 + 适配器"。升级 WebRTC 是**加一个适配器**，不动调用方。**用普通 hook 管理播放状态，不用 XState**（XState 仅限 Phase 1 的出价+连接两条流，勿过度套用）。

### 6.1 适配器接口（`features/live-media/adapters/types.ts`）

```ts
export interface PlaybackEnv {
  videoEl: HTMLVideoElement;
  canNativeHls: boolean;   // video.canPlayType('application/vnd.apple.mpegurl') !== ''
  mseHlsSupported: boolean;// Hls.isSupported()
}

export interface AdapterCallbacks {
  onPlaying(): void;
  onFatal(reason: string): void;   // 触发降级链推进
  onRecoverableError?(reason: string): void;
}

export interface MediaAdapterHandle {
  detach(): void;          // 必须幂等、必须释放 hls.js 实例 / 解绑 video.src
}

export interface MediaAdapter {
  readonly protocols: MediaProtocol[];        // 该适配器支持的协议
  canPlay(source: MediaSource, env: PlaybackEnv): boolean;
  attach(source: MediaSource, env: PlaybackEnv, cb: AdapterCallbacks): MediaAdapterHandle;
}
```

### 6.2 `mp4` 适配器（`adapters/mp4.ts`）

等价于 Phase 1 现状那行 `<video src>`：

```ts
// canPlay: 总是 true（兜底）
// attach: env.videoEl.src = source.url; videoEl.load(); 监听 'playing'→onPlaying, 'error'→onFatal
// detach: videoEl.removeAttribute('src'); videoEl.load();
```

保留 `muted + playsInline + autoPlay`（移动端自动播放必需），并在 `attach` 后 `videoEl.play().catch(...)`（自动播放被拒时不算 fatal，留待用户手势）。

### 6.3 `hls/ll-hls` 适配器（`adapters/hls.ts`）

覆盖 `protocol ∈ {'hls','ll-hls'}`，**两条路径**：

```ts
// canPlay(source, env): env.canNativeHls || env.mseHlsSupported

// attach:
//  路径 A（Safari/iOS 原生）：env.canNativeHls 为真 → videoEl.src = source.url; 监听 'playing'/'error'
//  路径 B（其他浏览器）：Hls.isSupported() → new Hls({ lowLatencyMode: true, ... });
//      hls.loadSource(source.url); hls.attachMedia(videoEl);
//      hls.on(Hls.Events.ERROR, (e, data) => {
//        if (data.fatal) {
//          switch (data.type) {
//            case Hls.ErrorTypes.NETWORK_ERROR: hls.startLoad(); // 先尝试自恢复
//            case Hls.ErrorTypes.MEDIA_ERROR:   hls.recoverMediaError();
//            default: cb.onFatal('hls fatal: ' + data.type); // 自恢复无效→降级链推进
//          }
//        } else cb.onRecoverableError?.(data.details);
//      });
// detach: hls.destroy()（路径 B）或 videoEl.removeAttribute('src')（路径 A）；务必幂等
```

hls.js 低延迟相关配置（为 Phase 3 LL-HLS 准备，本期对静态样例同样适用）：`lowLatencyMode: true`、合理的 `liveSyncDuration`/`liveMaxLatencyDuration`/`backBufferLength`。具体数值在 Phase 3 对真实流调优，本期用默认 + `lowLatencyMode` 即可。

> **依赖**：本期需引入 `hls.js`。Safari/iOS 走原生**不需要** hls.js，但桌面 Chrome/Firefox 需要。按需动态 import（`await import('hls.js')`）以免移动端 Safari 白白加载。

### 6.4 `whep` 适配器（`adapters/whep.ts`）——仅接缝

```ts
export const whepAdapter: MediaAdapter = {
  protocols: ['whep'],
  canPlay: () => false,                       // 本期恒不可用
  attach: () => { throw new Error('WHEP adapter not implemented in Phase 2'); },
};
```

注册进选择器但 `canPlay` 恒 false，确保即使描述符里出现 `whep` 也会被跳过、降级到下一个 source。**不写实现。**

### 6.5 选择器 + 降级链（`usePlaybackEngine.ts`）

普通 hook，维护：当前 source index、播放状态、降级推进。

```ts
type PlaybackStatus = 'idle' | 'connecting' | 'playing' | 'degraded' | 'exhausted';
// exhausted = 所有 source 都 fatal 过 → 停在 posterURL 静态图

// 逻辑：
//  1. 入参 MediaPlayback.sources 已按 priority 升序（= 降级链）。
//  2. env 探测：canNativeHls / mseHlsSupported（detect.ts）。
//  3. 从 index=0 起，找第一个【适配器存在且 canPlay】的 source，attach。
//  4. onFatal → detach 当前 → index++ → 重复 3；越过不可播协议（如 whep）。
//  5. 所有 source 耗尽 → status='exhausted' → 显示 posterURL（不再重试，不报错给竞拍）。
//  6. onPlaying → status='playing'。
//  7. 组件卸载 / auctionId 变化 → detach（幂等），防 hls.js 泄漏。
```

降级链 `ll-hls → hls → mp4 → 海报` 因此是"按 priority 顺序逐个 attach，fatal 即推进，最后落海报"的自然结果。

---

## 7. 接线 LiveStage / LiveBackdrop

### 7.1 `useLiveMediaSource` 升级（内部改，签名兼容）

Phase 1 返回 `LiveMediaSourceV0`；本期改为返回 `MediaPlayback`（经 TanStack Query 拉接口）：

```ts
// features/live-media/useLiveMediaSource.ts （Phase 2）
export function useLiveMediaSource(auctionId: string) {
  return useQuery({
    queryKey: ['live-session', auctionId],
    queryFn: () => fetchLiveSession(auctionId),  // entities/live-session/api.ts
    staleTime: 60_000,
    retry: 1,                  // 媒体描述符失败不必激进重试；失败就降级/留海报
    // 关键：这是【独立】query，失败【不抛进】竞拍子树（M4）
  });
}
```

> **解耦点（M4）**：这是独立的 HTTP query，**不**复用竞拍 WS，**不**用 `staleTime:Infinity`（那是竞拍 WS 缓存的策略）。它失败时 `useQuery` 返回 error 态，由 `<LiveBackdrop>` 自行消化为"留海报"，**不 throw**。

### 7.2 `<LiveBackdrop>` 升级

```tsx
// widgets/live-stage/LiveBackdrop.tsx （Phase 2）
function LiveBackdrop({ auctionId }: { auctionId: string }) {
  const { data: playback, isError } = useLiveMediaSource(auctionId);
  const videoRef = useRef<HTMLVideoElement>(null);
  const { status } = usePlaybackEngine(videoRef, playback);  // 适配器选择 + 降级链

  // 始终渲染 <video> 元素（适配器往里 attach）；poster 兜底
  return (
    <video
      ref={videoRef}
      className="live-video-bg"
      poster={playback?.posterURL || demoProductImageURL}
      autoPlay muted loop={!playback?.isLive} playsInline aria-hidden="true"
    />
    // status==='exhausted' || isError → 仅剩 poster 静态图，竞拍页其余照常
  );
}
```

`LiveStage` 调用方基本不动（Phase 1 已是 `const media = useLiveMediaSource(...)` + `<LiveBackdrop .../>` 形态；本期把 props 从 V0 source 改为传 `auctionId` 或新 `playback`，是局部改动）。

> `loop`：demo MP4（`isLive:false`）需要 `loop` 循环；真直播（`isLive:true`）不 loop。由 `isLive` 驱动。

---

## 8. 解耦验证（本期重心）

> 这是 Phase 2 最重要的交付——**不是"播放好看"，而是"媒体能独立崩溃而竞拍毫发无伤"**。把下列场景写成自动化测试 + 真机走查清单。

### 8.1 隔离测试矩阵（M2/M3/M4）

| 场景 | 注入方式 | 期望（竞拍侧必须全部正常） |
|---|---|---|
| 媒体接口 404 | mock `GET /api/live/sessions/{id}` 返回 404 | 价格/倒计时/出价/确认/恢复/氛围门控正常；画面只剩 poster |
| 流地址不可达 | source.url 指向死链 | 适配器 fatal → 降级链推进 → 最终 poster；竞拍无影响 |
| m3u8 损坏 | 本地样例改坏 | hls.js fatal → 降级 mp4 → 若也坏则 poster；竞拍无影响 |
| 媒体 query 持续 pending | 延迟响应 | 竞拍页不被阻塞渲染（媒体区独立 loading/poster），出价可正常进行 |
| 自动播放被拒 | 非用户手势环境 | 不算 fatal、不降级、不报错；保留 muted 重试或留 poster；竞拍无影响 |
| auctionId 切换 | 切换拍品 | 旧适配器 `detach()` 干净释放（无 hls.js 实例泄漏）、新描述符重新拉取 |

### 8.2 时钟解耦（M3）

- 断言倒计时数值在"视频卡顿/缓冲/暂停/快进"时**不变化**——它只跟 `deriveCountdown(服务端时间锚)` 走。
- code review：搜索整个媒体子树，确认**没有**任何地方用 `video.currentTime`/`buffered`/`readyState` 去推断竞拍价格、剩余时间或落槌。

### 8.3 错误边界（M2）

- `features/live-media` 外包一层 error boundary（或 hook 内吞错），保证媒体任何异常→降级/海报，**绝不冒泡**到竞拍页导致白屏。
- 测试：在适配器 `attach` 里强制 throw，确认竞拍页不崩。

**WP 验收（解耦）**：§8.1 矩阵全部通过；§8.2 时钟解耦断言通过；§8.3 错误不冒泡；Phase 1 I1–I7 仍绿。

---

## 9. 本地 LL-HLS 样例验证（不依赖 MediaMTX）

> Phase 2 要证明 hls.js / Safari 原生两条路径**现在就能跑**，但**不引入流媒体服务器**（那是 Phase 3）。用 ffmpeg **离线**把一段 MP4 切成静态 LL-HLS 分片，当普通静态文件伺服即可。

### 9.1 离线产样例（一次性，非运行时）

```bash
# 离线把 demo MP4 切成 HLS 分片（开发期一次性生成，产物当静态资源放）
ffmpeg -i jade-live-loop.mp4 \
  -c copy -f hls \
  -hls_time 2 -hls_list_size 6 -hls_flags delete_segments+append_list \
  -hls_segment_filename 'frontend/mobile-h5/public/demo/sample-llhls/seg_%03d.ts' \
  frontend/mobile-h5/public/demo/sample-llhls/index.m3u8
```

放进 `public/demo/sample-llhls/`，即可用 `/demo/sample-llhls/index.m3u8` 当一个 `protocol:'hls'` 的 source 来验证适配器。

### 9.2 验证清单

- **桌面 Chrome/Firefox（hls.js 路径）**：让描述符返回 `{protocol:'hls', url:'/demo/sample-llhls/index.m3u8'}`，确认 hls.js 拉起播放。
- **iOS Safari / macOS Safari（原生路径）**：同一描述符，确认走 `video.src` 原生 HLS 播放（不加载 hls.js）。
- **降级链**：描述符给 `[{hls, 坏链, p10}, {mp4, demo, p90}]`，确认 hls fatal 后自动降到 mp4。
- **移动端自动播放**：`muted+playsInline+autoPlay` 在真机 iOS Safari + Android Chrome 成功起播。

> 本地样例仅用于**开发期验证适配器**，不进 Phase 2 的"演示路径"（演示仍放循环 MP4）。它证明 Phase 3 接 MediaMTX 后前端可零改动消费 m3u8。

---

## 10. WP 执行顺序

| WP | 名称 | 依赖 | 产出 | 章节 |
|---|---|---|---|---|
| **WP-1** | 契约定型 + schema 守卫 | Phase 1 完成 | `shared/media/contract.ts`、键集合断言单测 | [§4](#4-mediaplayback-契约定型) |
| **WP-2** | 后端 Live Session API | WP-1 | `LiveSession` handler + 路由 + config 字段 + 只读/无竞拍真相单测 | [§5](#5-后端live-session-api) |
| **WP-3** | 适配器系统 | WP-1 | `MediaAdapter` 接口 + mp4/hls/whep(接缝) + detect.ts | [§6](#6-前端媒体源适配器系统) |
| **WP-4** | 接线 + 降级链 | WP-2,WP-3 | `useLiveMediaSource`(改) + `usePlaybackEngine` + `<LiveBackdrop>`(改) | [§7](#7-接线-livestage--livebackdrop) |
| **WP-5** | 解耦验证 | WP-4 | §8 隔离测试矩阵 + 时钟解耦 + 错误边界 | [§8](#8-解耦验证本期重心) |
| **WP-6** | 本地 LL-HLS 样例验证 | WP-4 | ffmpeg 样例 + hls.js/Safari 双路径走查 | [§9](#9-本地-ll-hls-样例验证不依赖-mediamtx) |

顺序：先定契约（WP-1）→ 前后端并行（WP-2 后端 / WP-3 前端适配器）→ 接线（WP-4）→ 解耦验证（WP-5，重心）→ 样例验证（WP-6）。

---

## 11. 验收与开放问题

### 11.1 验收标准

- **契约**：`MediaPlayback` 定型；schema 守卫单测确保响应不含竞拍字段（M1）。
- **后端**：`GET /api/live/sessions/{id}` 上线、只读、config 驱动；demo 返回 MP4 描述符；handler 单测断言无竞拍真相、无副作用；未改竞拍/结算/恢复、未加 DB 列。
- **前端**：`LiveStage`/`LiveBackdrop` **无任何写死流地址/协议分支**；切换描述符即可换源；`whep` 接缝存在但 `canPlay` 恒 false。
- **降级链**：`ll-hls→hls→mp4→海报` 可触发（构造坏链验证）。
- **解耦（核心）**：§8.1 矩阵全通过；倒计时不被视频状态影响（M3）；媒体错误不冒泡（M2/M3/M4）；Phase 1 I1–I7 全绿。
- **双路径**：桌面 hls.js + Safari 原生 HLS 均拉起本地样例；移动端 `muted+playsInline+autoPlay` 起播成功。
- **无 Phase 3 内容**：未部署 MediaMTX、未实现 WebRTC。

### 11.2 开放问题（需团队/用户确认）

1. **接口鉴权**：放认证组（默认，推荐）还是免登录直链（demo 便利）？影响 Phase 3 防盗链设计。
2. **样例/流文件托管位置**：demo MP4 与 LL-HLS 样例放前端 `public/`（现状 `/demo/...`）、后端静态目录、还是经 `/api/media` 代理？Phase 3 MediaMTX 出口又如何反代到同源？建议本期沿用 `public/`，Phase 3 再定反代。
3. **posterURL 来源**：用 auction→item 的 `ImageURL`（经 `/api/media`）还是恒空？是否值得在 handler 里多查一次 item？
4. **`sessionEpoch` 是否本期就填**：Phase 3 流重启后需要 cache-busting；本期可留空，但要确认前端 query key 是否预留它。
5. **hls.js 低延迟参数**：`liveSyncDuration` 等的具体值留到 Phase 3 对真实流调优；本期是否只用 `lowLatencyMode:true` 默认即可（建议是）。

---

## 12. 留给 Phase 3 及以后的口子

| 口子 | 本期状态 | Phase 3 / Future 要做 |
|---|---|---|
| `GET /api/live/sessions/{id}` | config 返回 MP4 描述符 | 改 config → 返回 MediaMTX LL-HLS m3u8 描述符（**Go 逻辑零改**） |
| `MediaPlayback.sources` 降级链 | 单一 mp4 source | 填 `[ll-hls, hls, mp4]` 多级降级 |
| `hls/ll-hls` 适配器 | 已实现，本地样例验证 | 对接 MediaMTX 真实 m3u8 + 低延迟参数调优 |
| `whep` 适配器 | 空接缝（canPlay=false） | Future：实现 WHEP/WebRTC 亚秒拉流，调用方不变 |
| `capabilities.webrtc` | 恒 false | Future：真探测 + WHEP 协商 |
| `sessionEpoch` | 预留字段 | Phase 3：流重启 cache-busting / 重新拉描述符 |
| 反代/HTTPS/同源 | 未涉及 | Phase 3：MediaMTX 出口经 443 反代、避免混合内容 |

> 口子原则：**接口形状现在就对，内部实现 Phase 3 填**。Phase 3 是"后端换 source + 部署 MediaMTX"，前端因 Phase 2 的适配器已就绪而**零改动**。

---

## 13. 不要做清单（Consolidated Don'ts）

- 不把任何竞拍真相（price/winner/status/seq/终态/结算）放进 Live Session API（M1）。
- 不让媒体失败影响竞拍：不共享 WS、不共享错误边界、不阻塞渲染、不冒泡异常（M2/M4）。
- 不用视频 `currentTime`/缓冲态推断竞拍时间/价格/落槌（M3）；倒计时只认 `deriveCountdown`。
- 不在前端写死流地址或写死协议分支（一律经契约 + 适配器）。
- 不实现 WebRTC/WHEP（仅留接缝）；不部署 MediaMTX（Phase 3）；不做多码率/CDN/防盗链。
- 不改后端竞拍/结算/恢复代码、不改 WS 协议、不加 DB 列、不动 `displayMediaURL`。
- 不给媒体播放套 XState（用普通 hook；XState 仅限出价+连接）。
- 不让 hls.js 实例泄漏（`detach` 必须 `hls.destroy()` 且幂等）。

---

## 14. PR 切分建议

1. `WP-1` 契约 + schema 守卫单测（纯类型 + 测试，零运行时影响）。
2. `WP-2` 后端 Live Session API（纯新增只读接口，独立可回滚）。
3. `WP-3` 适配器系统（前端纯新增模块，未接线前不影响现状）。
4. `WP-4` 接线 + 降级链（把 `<LiveBackdrop>` 从 V0 切到契约——**此 PR 改变运行时行为，单独评审**）。
5. `WP-5` 解耦验证（测试 + 错误边界，核心正确性，单独评审）。
6. `WP-6` 本地样例 + 双路径走查（含 ffmpeg 产物与文档）。

每个 PR 描述必须含：涉及哪条红线（M1–M4 / I1–I7）、如何验证未回归、是否改变运行时行为、回滚影响面。

---

## 附：本手册引用的真实代码锚点（已核验）

- 路由（chi，集中装配）：`backend/internal/gateway/router.go`（认证组 `r.Group`+`authMiddleware` 在 `:130`–`:199`；`GetAuction` 注册于 `:139`；媒体代理 `r.Get("/media/*", auctionHandler.ServeMedia)` 在认证组外 `:129`）。新增 `r.Get("/live/sessions/{id}", auctionHandler.LiveSession)` 入认证组。
- Handler 范式：`backend/internal/gateway/auction_handlers.go`（`GetAuction` `:644`、`ServeMedia` `:397`——后者是"伺服媒体但不持竞拍真相"的先例：校验路径、`..` 防穿越、`items/` 前缀、content-type + cache 头）。
- 数据模型（关键事实）：`backend/internal/auction/model.go`（`Item` 仅 `ImageURL` `:19`，**无 video 列** → 描述符 config 驱动）；`Auction` `:48`（含 price/winner/status，**这些禁止进媒体接口**）。
- config 范式：`backend/internal/config/config.go`（按现有 env 读取惯例新增 §5.3 字段）。
- 前端媒体口子（Phase 1 产物，本期升级）：`frontend/mobile-h5/src/components.tsx`（`LiveStage` 内 `LiveBackdrop`；Phase 1 已把 `:129 const videoURL = demoLiveVideoURL` / `:185 <video className="live-video-bg" …>` 收敛进 `features/live-media`）；`frontend/mobile-h5/src/domain.ts:392`（`demoLiveVideoURL`）、`:393`（`demoProductImageURL`）、`:4`（`displayMediaURL`，封面改写，**本期不动**）。
- 竞拍真相/时钟（不得被媒体污染）：`frontend/mobile-h5/src/domain.ts`（`deriveCountdown` 服务端时间锚定，I4）；竞拍 WS 与恢复在 `frontend/mobile-h5/src/main.tsx`。
- 栈现状：React 18.3.1 + Vite 5.3.3 + TS 5.5.3；本期前端新增 `hls.js`（按需动态 import）；TanStack Query 已在 Phase 1 引入。
