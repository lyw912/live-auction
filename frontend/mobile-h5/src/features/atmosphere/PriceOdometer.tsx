import React, { useEffect, useRef, useState } from 'react';
import { formatCents } from '../../domain';

export function PriceOdometer({
  valueCents,
  className,
  reducedMotion = false
}: {
  valueCents: number;
  className?: string;
  reducedMotion?: boolean;
}) {
  const previousValueRef = useRef(valueCents);
  const frameRef = useRef<number | null>(null);
  const [displayValue, setDisplayValue] = useState(valueCents);
  const direction = valueCents > previousValueRef.current ? 'up' : valueCents < previousValueRef.current ? 'down' : 'still';

  useEffect(() => {
    if (frameRef.current != null) {
      window.cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    }
    const from = previousValueRef.current;
    const to = valueCents;
    previousValueRef.current = valueCents;
    if (reducedMotion || from === to) {
      setDisplayValue(to);
      return;
    }
    const duration = 320;
    const startedAt = performance.now();
    const tick = (now: number) => {
      const progress = Math.min(1, (now - startedAt) / duration);
      const eased = 1 - Math.pow(1 - progress, 3);
      setDisplayValue(Math.round(from + (to - from) * eased));
      if (progress < 1) {
        frameRef.current = window.requestAnimationFrame(tick);
      }
    };
    frameRef.current = window.requestAnimationFrame(tick);
    return () => {
      if (frameRef.current != null) window.cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    };
  }, [reducedMotion, valueCents]);

  return (
    <span className={className} data-odometer-direction={direction} aria-label={`当前最高价 ${formatCents(valueCents)}`}>
      {formatCents(displayValue)}
    </span>
  );
}
