import { useState } from 'react';
import { AlertTriangle, Clock3, Copy, Download, Trophy } from 'lucide-react';
import type { AuctionItem, AuctionSummary, BottomSheetKey, HeatSnapshot, PaymentPhase, ResultRecap, ResultSheetKind, Scenario } from './domain';
import { auctionStatusLabel, buildHighlightCard, buildResultRecap, formatCents } from './domain';

function displayOrderNo(orderID: string) {
  if (!orderID) return 'JP待生成';
  const compact = orderID.replace(/^ord[_-]?/i, '').replace(/[^a-z0-9]/gi, '').slice(-8).toUpperCase();
  return `JP${new Date().toISOString().slice(0, 10).replace(/-/g, '')}-${compact || '00000000'}`;
}

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const input = document.createElement('textarea');
  input.value = value;
  input.setAttribute('readonly', 'true');
  input.style.position = 'fixed';
  input.style.left = '-9999px';
  document.body.appendChild(input);
  input.select();
  const ok = document.execCommand('copy');
  input.remove();
  if (!ok) throw new Error('copy failed');
}

async function buildHighlightVideo(recap: ResultRecap): Promise<{ blob: Blob; filename: string }> {
  const canvas = document.createElement('canvas');
  canvas.width = 720;
  canvas.height = 1280;
  const ctx = canvas.getContext('2d');
  const capture = canvas.captureStream?.bind(canvas);
  if (!ctx || !capture || typeof MediaRecorder === 'undefined') {
    throw new Error('highlight video unsupported');
  }
  const mimeType = MediaRecorder.isTypeSupported('video/webm;codecs=vp9')
    ? 'video/webm;codecs=vp9'
    : MediaRecorder.isTypeSupported('video/webm;codecs=vp8')
      ? 'video/webm;codecs=vp8'
      : 'video/webm';
  const stream = capture(30);
  const chunks: BlobPart[] = [];
  const recorder = new MediaRecorder(stream, { mimeType });
  const done = new Promise<Blob>((resolve, reject) => {
    recorder.ondataavailable = (event) => {
      if (event.data.size > 0) chunks.push(event.data);
    };
    recorder.onerror = () => reject(new Error('record highlight video'));
    recorder.onstop = () => resolve(new Blob(chunks, { type: mimeType }));
  });
  recorder.start();
  const startedAt = performance.now();
  const durationMS = 4200;
  await new Promise<void>((resolve) => {
    const drawFrame = (now: number) => {
      const progress = Math.min(1, (now - startedAt) / durationMS);
      drawHighlightFrame(ctx, recap, progress);
      if (progress < 1) {
        requestAnimationFrame(drawFrame);
      } else {
        resolve();
      }
    };
    requestAnimationFrame(drawFrame);
  });
  recorder.stop();
  stream.getTracks().forEach((track) => track.stop());
  const filenameTitle = recap.title
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 28) || 'auction-highlight';
  return { blob: await done, filename: `${filenameTitle}-highlight.webm` };
}

function drawHighlightFrame(ctx: CanvasRenderingContext2D, recap: ResultRecap, progress: number) {
  const width = ctx.canvas.width;
  const height = ctx.canvas.height;
  const gradient = ctx.createLinearGradient(0, 0, width, height);
  gradient.addColorStop(0, '#101827');
  gradient.addColorStop(0.44, '#14433b');
  gradient.addColorStop(1, '#d18a2f');
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, width, height);
  ctx.save();
  ctx.globalAlpha = 0.18;
  ctx.fillStyle = '#ffffff';
  for (let index = 0; index < 18; index += 1) {
    const x = (index * 97 + progress * 180) % (width + 120) - 60;
    const y = (index * 139 + progress * 260) % (height + 120) - 60;
    ctx.beginPath();
    ctx.arc(x, y, index % 3 === 0 ? 18 : 10, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.restore();
  roundedRect(ctx, 48, 70, width - 96, height - 140, 30, 'rgba(255,255,255,0.13)', 'rgba(255,255,255,0.28)');
  ctx.fillStyle = '#ffe6a7';
  ctx.font = '700 34px Arial, sans-serif';
  ctx.fillText(recap.status, 82, 150);
  ctx.fillStyle = '#ffffff';
  ctx.font = '800 50px Arial, sans-serif';
  wrapCanvasText(ctx, recap.title, 82, 250, width - 164, 58, 2);
  const scale = 1 + Math.sin(progress * Math.PI) * 0.035;
  ctx.save();
  ctx.translate(82, 440);
  ctx.scale(scale, scale);
  ctx.fillStyle = '#ffffff';
  ctx.font = '900 78px Arial, sans-serif';
  ctx.fillText(recap.price, 0, 0);
  ctx.restore();
  ctx.fillStyle = '#fff0c7';
  ctx.font = '700 30px Arial, sans-serif';
  ctx.fillText(`成交/领先：${recap.winner}`, 82, 520);
  roundedRect(ctx, 82, 625, width - 164, 210, 22, 'rgba(255,255,255,0.15)');
  ctx.fillStyle = '#ffffff';
  ctx.font = '800 32px Arial, sans-serif';
  ctx.fillText('高光事实', 112, 690);
  ctx.fillStyle = '#fff5db';
  ctx.font = '400 28px Arial, sans-serif';
  wrapCanvasText(ctx, recap.facts.join(' · ') || '真实竞拍记录', 112, 755, width - 224, 38, 2);
  ctx.fillStyle = '#ffffff';
  ctx.font = '800 36px Arial, sans-serif';
  wrapCanvasText(ctx, recap.nextAction, 82, 975, width - 164, 44, 2);
  ctx.fillStyle = 'rgba(255,255,255,0.75)';
  ctx.font = '400 24px Arial, sans-serif';
  ctx.fillText('仅展示系统真实竞拍记录，用户身份已脱敏。', 82, 1158);
}

function roundedRect(ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number, fill: string, stroke?: string) {
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.arcTo(x + width, y, x + width, y + height, radius);
  ctx.arcTo(x + width, y + height, x, y + height, radius);
  ctx.arcTo(x, y + height, x, y, radius);
  ctx.arcTo(x, y, x + width, y, radius);
  ctx.closePath();
  ctx.fillStyle = fill;
  ctx.fill();
  if (stroke) {
    ctx.strokeStyle = stroke;
    ctx.lineWidth = 2;
    ctx.stroke();
  }
}

function wrapCanvasText(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, maxWidth: number, lineHeight: number, maxLines: number) {
  const chars = Array.from(text);
  let line = '';
  let lineCount = 0;
  for (const char of chars) {
    const next = line + char;
    if (ctx.measureText(next).width > maxWidth && line) {
      lineCount += 1;
      ctx.fillText(lineCount === maxLines ? `${line.slice(0, Math.max(0, line.length - 1))}...` : line, x, y);
      if (lineCount >= maxLines) return;
      y += lineHeight;
      line = char;
    } else {
      line = next;
    }
  }
  if (line && lineCount < maxLines) ctx.fillText(line, x, y);
}

export function ResultSheet({
  activeSheet,
  compact = false,
  heat,
  item,
  kind,
  nextAuction,
  orderAmountCents,
  orderID,
  paymentPhase,
  scenario,
  terminalPriceCents,
  terminalWinnerID,
  terminalWinnerMasked,
  userBestCents,
  onOpenOrders,
  onPay
}: {
  activeSheet: BottomSheetKey | null;
  compact?: boolean;
  heat?: HeatSnapshot;
  item: AuctionItem;
  kind: ResultSheetKind | null;
  nextAuction?: AuctionSummary;
  orderAmountCents: number;
  orderID: string;
  paymentPhase: PaymentPhase;
  scenario: Scenario;
  terminalPriceCents: number;
  terminalWinnerID: string;
  terminalWinnerMasked: string;
  userBestCents: number;
  onOpenOrders: () => void;
  onPay: () => void;
}) {
  const [shareFeedback, setShareFeedback] = useState('');
  if (!kind || activeSheet) return null;
  const soldPrice = formatCents(orderAmountCents || terminalPriceCents);
  const nextTitle = nextAuction?.item?.title ?? '下一件拍品';
  const nextPrice = nextAuction ? formatCents(nextAuction.current_price_cents ?? 0) : '';
  const nextStatus = nextAuction ? auctionStatusLabel(nextAuction.status) : '';
  const gapCents = Math.max(0, terminalPriceCents - userBestCents);
  const winnerDisplayName = terminalWinnerMasked || (kind === 'winner' ? '我' : '领先者');
  const isPaymentDisabled = scenario.ctaDisabled || paymentPhase === 'pending' || paymentPhase === 'paid' || paymentPhase === 'expired' || !orderID;
  const title = kind === 'winner'
    ? paymentPhase === 'paid'
      ? '支付已完成'
      : paymentPhase === 'expired'
        ? '支付窗口已关闭'
        : '恭喜中拍'
    : kind === 'loser'
      ? '本场已落槌'
      : '本场未成交';
  const recapHeat: HeatSnapshot = heat ?? {
    activeBidders30s: 0,
    acceptedBids30s: 0,
    priceVelocityCentsPerMin: 0,
    acceptedBidderCount: 0,
    totalAcceptedBids: 0,
    source: 'fallback'
  };
  const recap: ResultRecap = buildResultRecap({
    itemTitle: item.title ?? scenario.title,
    kind,
    terminalPriceCents: orderAmountCents || terminalPriceCents,
    terminalWinnerID,
    terminalWinnerMasked: winnerDisplayName,
    heat: recapHeat,
    nextTitle: nextAuction?.item?.title
  });
  const copyRecap = async () => {
    if (!recap) return;
    try {
      await copyText(recap.shareCopy);
      setShareFeedback('已复制');
    } catch {
      setShareFeedback('复制失败');
    }
  };
  const downloadHighlight = () => {
    if (!recap) return;
    const card = buildHighlightCard(recap);
    const blob = new Blob([card.content], { type: card.mimeType });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = card.filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 250);
    setShareFeedback('已保存');
  };
  const downloadHighlightVideo = async () => {
    if (!recap) return;
    try {
      const clip = await buildHighlightVideo(recap);
      const url = URL.createObjectURL(clip.blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = clip.filename;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      setShareFeedback('视频已保存');
    } catch {
      setShareFeedback('当前浏览器不支持视频生成');
    }
  };

  return (
    <section className={`result-sheet ${kind} ${compact ? 'is-compact' : ''}`} data-testid="result-sheet" aria-label={title}>
      {!compact ? <div className="result-cinematic-bg" aria-hidden="true" /> : null}
      {!compact ? <div className="result-confetti" aria-hidden="true"><span /><span /><span /><span /><span /></div> : null}
      <div className="result-sheet-icon" aria-hidden="true">
        {kind === 'winner' ? <Trophy size={22} /> : kind === 'loser' ? <Clock3 size={22} /> : <AlertTriangle size={22} />}
      </div>
      <div className="result-sheet-copy">
        <p className="result-eyebrow">{kind === 'winner' ? '成交结果' : kind === 'loser' ? '输家承接' : '未成交说明'}</p>
        <h2>{title}</h2>
        {kind === 'winner' && (
          <>
            <p>成交价 {soldPrice}。订单 {displayOrderNo(orderID)} 已锁定，支付状态：{paymentPhase === 'paid' ? '已支付' : paymentPhase === 'pending' ? '确认中' : paymentPhase === 'expired' ? '已超时' : '待支付'}。</p>
            <p>保证金会随订单状态处理；支付成功后订单完成，未支付超时会关闭支付窗口。</p>
          </>
        )}
        {kind === 'loser' && (
          <>
            <p>{winnerDisplayName} 以 {formatCents(terminalPriceCents)} 中拍。{gapCents > 0 ? `你距离成交差 ${formatCents(gapCents)}。` : '你未在最后价格领先。'}</p>
            <p>可继续关注 {nextTitle}，本场历史会保留在出价记录中。下一件来自当前直播间拍品列表，不是库存预留或个性化推荐。</p>
          </>
        )}
        {kind === 'unsold' && (
          <>
            <p>本场没有形成有效成交，出价入口已关闭，不会生成订单。</p>
            <p>{nextAuction ? `${nextTitle} 即将开始，可回到商品列表继续观看。` : '暂无下一件排期，稍后回到直播间。'}</p>
          </>
        )}
      </div>
      {recap ? (
        <div className="result-recap-card" data-testid="h5-result-recap-card">
          <span>{recap.status}</span>
          <strong>{recap.title}</strong>
          <div>
            <em>{recap.price}</em>
            <em>{recap.winner}</em>
          </div>
          <p>{recap.facts.length ? recap.facts.join(' · ') : '只展示真实竞拍记录'}</p>
          <small>{recap.nextAction}</small>
          <div className="result-recap-actions">
            <button type="button" aria-label="copy-result-recap" onClick={() => void copyRecap()}>
              <Copy size={14} /> 复制
            </button>
            <button type="button" aria-label="download-highlight-card" onClick={downloadHighlight}>
              <Download size={14} /> 高光卡
            </button>
            <button type="button" aria-label="download-highlight-video" onClick={() => void downloadHighlightVideo()}>
              <Download size={14} /> 短视频
            </button>
            {shareFeedback ? <b>{shareFeedback}</b> : null}
          </div>
        </div>
      ) : null}
      {kind === 'winner' ? (
        <div className="result-climax-card" data-testid="result-climax-card" aria-label="落槌高光">
          <span>落槌高光</span>
          <strong>{soldPrice}</strong>
          <p>{Math.max(0, recapHeat.acceptedBidderCount)} 人有效出价 · {Math.max(0, recapHeat.totalAcceptedBids ?? recapHeat.acceptedBids30s)} 次真实出价</p>
          <em>战绩卡已生成，先确认成交事实再进入支付</em>
        </div>
      ) : null}
      {kind !== 'winner' && nextAuction ? (
        <div className="next-auction-card" data-testid="next-auction-handoff">
          <span>直播间下一件</span>
          <strong>{nextTitle}</strong>
          <p>{nextStatus} · 当前/起拍 {nextPrice}</p>
          <small>仅展示同直播间下一件可见拍品；未承诺相似度、库存预留或中标优先权。</small>
        </div>
      ) : null}
      <div className="result-actions">
        {kind === 'winner' ? (
          <>
            <button type="button" data-testid="result-pay-cta" disabled={isPaymentDisabled} onClick={onPay}>
              {paymentPhase === 'paid' ? '已支付' : paymentPhase === 'pending' ? '支付确认中' : paymentPhase === 'expired' ? '已超时' : '立即支付'}
            </button>
            <button type="button" onClick={onOpenOrders}>查看订单</button>
          </>
        ) : (
          <>
            <button type="button" onClick={onOpenOrders}>{kind === 'loser' ? '查看出价记录' : '查看商品列表'}</button>
            <span>{nextAuction ? `下一件：${nextTitle}` : '等待主播切换下一件'}</span>
          </>
        )}
      </div>
    </section>
  );
}
