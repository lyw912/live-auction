import React, { Suspense } from 'react';
import type { CommandVizFreshnessState, CommandVizPoint } from './CommandVizStrip';

const CommandVizStripImpl = React.lazy(() => import('./CommandVizStrip').then((module) => ({ default: module.CommandVizStrip })));

export function CommandVizStripShell({
  points,
  freshnessState,
  freshnessText
}: {
  points: CommandVizPoint[];
  freshnessState: CommandVizFreshnessState;
  freshnessText: string;
}) {
  return (
    <Suspense fallback={<div className="command-viz-skeleton" aria-label="控制室图表加载中" />}>
      <CommandVizStripImpl points={points} freshnessState={freshnessState} freshnessText={freshnessText} />
    </Suspense>
  );
}
