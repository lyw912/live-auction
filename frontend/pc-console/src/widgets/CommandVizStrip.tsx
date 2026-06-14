import React, { useEffect, useMemo, useRef } from 'react';
import * as echarts from 'echarts/core';
import { LineChart as ELineChart } from 'echarts/charts';
import { GridComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import { Area, AreaChart, ResponsiveContainer } from 'recharts';
import { Group } from '@visx/group';
import { scaleLinear, scaleTime } from '@visx/scale';
import { LinePath } from '@visx/shape';

echarts.use([ELineChart, GridComponent, TooltipComponent, CanvasRenderer]);

export type CommandVizPoint = {
  time: number;
  accepted: number;
  rejected: number;
  latencyMS: number;
};

export type CommandVizFreshnessState = 'live' | 'stale' | 'paused';

function chartColor(name: string, fallback: string) {
  if (typeof window === 'undefined') return fallback;
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return value ? `hsl(${value})` : fallback;
}

function freshnessLabel(state: CommandVizFreshnessState) {
  if (state === 'stale') return 'Stale';
  if (state === 'paused') return 'Paused';
  return 'Live';
}

export function CommandVizStrip({
  points,
  freshnessState,
  freshnessText
}: {
  points: CommandVizPoint[];
  freshnessState: CommandVizFreshnessState;
  freshnessText: string;
}) {
  const windowedPoints = useMemo(() => points.slice(-120), [points]);
  return (
    <section className="command-viz-strip" data-testid="pc-command-viz-strip" aria-label="控制室实时可视化">
      <div className="command-viz-card">
        <span>Bid Rate</span>
        <strong>{windowedPoints[windowedPoints.length - 1]?.accepted ?? 0}/s</strong>
        <MiniSparkline points={windowedPoints} />
      </div>
      <div className="command-viz-card command-viz-wide">
        <span>Realtime Window</span>
        <EChartsWindow points={windowedPoints} />
      </div>
      <div className="command-viz-card">
        <span>Flight Path</span>
        <VisxTimeline points={windowedPoints} />
      </div>
      <div className="command-viz-freshness" aria-live="polite">
        <span data-state={freshnessState}>{freshnessLabel(freshnessState)}</span>
        <em>{freshnessText}</em>
      </div>
    </section>
  );
}

function MiniSparkline({ points }: { points: CommandVizPoint[] }) {
  const data = points.map((point) => ({ value: point.accepted, time: point.time }));
  return (
    <ResponsiveContainer width="100%" height={42}>
      <AreaChart data={data}>
        <Area type="monotone" dataKey="value" stroke={chartColor('--chart-1', '#22d3ee')} fill={chartColor('--chart-1', '#22d3ee')} fillOpacity={0.16} strokeWidth={2} isAnimationActive={false} />
      </AreaChart>
    </ResponsiveContainer>
  );
}

function EChartsWindow({ points }: { points: CommandVizPoint[] }) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!ref.current) return;
    const chart = echarts.init(ref.current, undefined, { renderer: 'canvas' });
    chart.setOption({
      animation: false,
      grid: { left: 24, right: 8, top: 8, bottom: 20 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: points.map((point) => new Date(point.time).toLocaleTimeString()), axisLabel: { color: 'rgba(226,232,240,.62)', fontSize: 10 } },
      yAxis: { type: 'value', axisLabel: { color: 'rgba(226,232,240,.62)', fontSize: 10 }, splitLine: { lineStyle: { color: 'rgba(148,163,184,.16)' } } },
      series: [
        { type: 'line', name: 'accepted', data: points.map((point) => point.accepted), showSymbol: false, lineStyle: { color: chartColor('--chart-1', '#22d3ee'), width: 2 } },
        { type: 'line', name: 'rejected', data: points.map((point) => point.rejected), showSymbol: false, lineStyle: { color: chartColor('--chart-4', '#f43f5e'), width: 2 } }
      ]
    });
    const resize = () => chart.resize();
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      chart.dispose();
    };
  }, [points]);

  return <div ref={ref} className="command-echarts-window" aria-hidden="true" />;
}

function VisxTimeline({ points }: { points: CommandVizPoint[] }) {
  const width = 260;
  const height = 42;
  const x = scaleTime({
    domain: [new Date(points[0]?.time ?? Date.now()), new Date(points[points.length - 1]?.time ?? Date.now())],
    range: [0, width]
  });
  const y = scaleLinear({
    domain: [0, Math.max(1, ...points.map((point) => point.latencyMS))],
    range: [height, 2]
  });
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="command-visx-timeline" role="img" aria-label="决策延迟时间线">
      <Group>
        <LinePath
          data={points}
          x={(point) => x(new Date(point.time))}
          y={(point) => y(point.latencyMS)}
          stroke={chartColor('--chart-2', '#fbbf24')}
          strokeWidth={2}
        />
      </Group>
    </svg>
  );
}
