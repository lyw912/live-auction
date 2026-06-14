# 前端闭环：PC 主播控制台

父文档：[产品范围](../00-project/01-product-scope.md)
相关文档：[AI 与运营](../03-backend/04-ai-ops-closed-loop.md)、[监控运维](../06-observability/00-ops-observability.md)

## PC 控制台职责

PC 端不是营销页，而是主播/商家的作业台：

- 商品上传与 AI 选品草稿；
- 拍品创建、规则配置、排期、启动、取消；
- PC 摄像头采集并通过 WHIP 推流到 MediaMTX；
- 活跃拍品队列、订单和支付状态；
- AI 解说、热度摘要、哨兵告警、复盘；
- 飞行记录器和监控面板。

## 图 5-2-1：主播工作台闭环





<!-- excalidraw-generated:start -->
<div class="excalidraw-diagram" style="overflow-x: auto; width: 100%; margin: 12px 0 18px 0; padding: 12px 12px 14px 12px; border: 1px solid #e2e8f0; border-radius: 8px; background: #fffdf7;">
  <a href="../assets/excalidraw/05-frontend-02-pc-console-closed-loop-01.excalidraw">打开可编辑 Excalidraw 源文件</a>
  <br />
  <img src="../assets/excalidraw/05-frontend-02-pc-console-closed-loop-01.svg" alt="图 5-2-1：主播工作台闭环" loading="lazy" width="1424" style="display: block; width: 1424px; max-width: none !important; height: auto;" />
</div>
<!-- excalidraw-generated:end -->

这张图展示 PC 端不是“后台 CRUD”，而是从上架、排期、启动、监控、AI 辅助到诊断复盘的主播作业闭环。交易权威仍在后端，PC 端只发起操作和展示后端结果。

## API 闭环

| 工作流 | 路由 |
|---|---|
| 商品上传 | `/api/items/upload-url`, `/api/items/upload`, `/api/items` |
| 拍品配置 | `/api/auctions`, `/api/auctions/{id}/rules`, `/schedule`, `/start`, `/cancel` |
| AI 选品 | `/api/host/ai/listing-drafts`, `/apply` |
| AI 解说 | `/api/host/auctions/{id}/commentary` |
| 风险与复盘 | `/sentinel-alerts`, `/sentinel-evaluate`, `/recap` |
| 监控 | `/api/monitor/auctions`, `/flight-recorder`, `/outbox`, `/scheduler`, `/redis-engine` |
| 直播推流 | getUserMedia -> WHIP `/mtx/auction-live/whip` |
| 订单支付 | `/api/orders`, 支付事件字段来自 `payment_events` |

## 本轮演示前新增/修复

| 能力 | 说明 |
|---|---|
| 智能草稿更稳 | AI relay 返回 code fence、说明文字或残缺 JSON 时，后端会抽取首个 JSON object；视觉模型失败时 fallback 到文本草稿并标记 safety。 |
| 商品类目中文化 | 默认类目从 `collectibles` 调整为中文展示口径，减少演示时的英文杂项。 |
| 图片上传同源化 | PC 上传商品图片走 `/api/items/upload`，避免 MinIO presigned URL 暴露 `localhost:9000` 导致公网浏览器失败。 |
| 直播助手收敛 | 演示助手保留“对手压过买家”核心功能，避免无关按钮干扰录屏。 |
| PC/H5 身份隔离 | PC `5277` 使用 `la_session_pc`，H5 `5276` 使用 `la_session_h5`，防止同公网 IP 下 cookie 串号。 |

## 直播推流注意事项

PC 端摄像头预览不等于 H5 已收到直播。完整链路还要满足：

- 页面是 HTTPS 安全上下文；
- `/mtx/auction-live/whip` 可访问；
- MediaMTX WebRTC ICE 端口 `8189/udp` 或 TCP 路径可达；
- H5 端 WHEP 播放拿到 answer 并进入 connected。

浏览器摄像头版本适合演示“端到端低延迟直播闭环”。如果要追求更高画质和更稳定码率，可以扩展 OBS/WHIP 作为专业推流端，但当前录屏版本保持浏览器直采，降低演示操作成本。

## 图 5-2-2：PC 到后端的数据流





<!-- excalidraw-generated:start -->
<div class="excalidraw-diagram" style="overflow-x: auto; width: 100%; margin: 12px 0 18px 0; padding: 12px 12px 14px 12px; border: 1px solid #e2e8f0; border-radius: 8px; background: #fffdf7;">
  <a href="../assets/excalidraw/05-frontend-02-pc-console-closed-loop-02.excalidraw">打开可编辑 Excalidraw 源文件</a>
  <br />
  <img src="../assets/excalidraw/05-frontend-02-pc-console-closed-loop-02.svg" alt="图 5-2-2：PC 到后端的数据流" loading="lazy" width="1424" style="display: block; width: 1424px; max-width: none !important; height: auto;" />
</div>
<!-- excalidraw-generated:end -->

## 评委拷问

| 问题 | 回答 |
|---|---|
| 主播改规则会不会影响正在竞拍用户？ | 排期/启动后规则冻结，热路径使用服务端规则；PC 端错误会展示后端权威拒绝。 |
| 诊断面板是不是静态假数据？ | 监控路由来自 `MonitorHandler` 查询 PG/Redis/outbox/scheduler，不是纯前端 mock。 |
| AI 选品会不会直接发布假信息？ | listing draft 要 host apply，且 `human_review_required/no_auto_publish` 写入 safety。 |
| 为什么支付曾经卡住？ | PC/H5 共公网 IP 时 cookie 串号，H5 带 host session 支付被后端 403；现在用端口作用域 cookie 解决。 |
