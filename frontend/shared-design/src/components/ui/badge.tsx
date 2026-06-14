import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

export const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-[calc(var(--radius)-4px)] border px-2 py-0.5 text-xs font-semibold tabular-nums transition-colors',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary/12 text-primary',
        secondary: 'border-border bg-secondary text-secondary-foreground',
        live: 'border-live/30 bg-live/12 text-live',
        stale: 'border-stale/35 bg-stale/12 text-stale',
        paused: 'border-paused/35 bg-paused/12 text-paused',
        leading: 'border-state-leading/35 bg-state-leading/12 text-state-leading',
        outbid: 'border-state-outbid/35 bg-state-outbid/12 text-state-outbid',
        won: 'border-state-won/35 bg-state-won/12 text-state-won',
        lost: 'border-state-lost/35 bg-state-lost/12 text-state-lost',
        destructive: 'border-destructive/35 bg-destructive/12 text-destructive',
        outline: 'border-border text-foreground'
      }
    },
    defaultVariants: {
      variant: 'default'
    }
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}
