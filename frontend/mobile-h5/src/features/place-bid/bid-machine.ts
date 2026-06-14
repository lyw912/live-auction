import { setup } from 'xstate';
import type { BidPhase } from '../../domain';

export const bidMachine = setup({
  types: {
    context: {} as Record<string, never>,
    events: {} as
      | { type: 'SUBMIT' }
      | { type: 'ACCEPT' }
      | { type: 'REJECT' }
      | { type: 'CONFIRM_REQUIRED' }
      | { type: 'CONFIRM' }
      | { type: 'OUTBID' }
      | { type: 'UNCERTAIN' }
      | { type: 'RESET' }
  }
}).createMachine({
  id: 'h5Bid',
  context: {},
  initial: 'idle',
  states: {
    idle: { on: { SUBMIT: 'pending', OUTBID: 'outbid' } },
    pending: { on: { ACCEPT: 'accepted', REJECT: 'rejected', CONFIRM_REQUIRED: 'confirmRequired', UNCERTAIN: 'uncertain', OUTBID: 'outbid' } },
    confirmRequired: { on: { CONFIRM: 'confirming', RESET: 'idle' } },
    confirming: { on: { ACCEPT: 'accepted', REJECT: 'rejected', UNCERTAIN: 'uncertain' } },
    accepted: { on: { OUTBID: 'outbid', RESET: 'idle' } },
    rejected: { on: { SUBMIT: 'pending', RESET: 'idle' } },
    outbid: { on: { SUBMIT: 'pending', RESET: 'idle' } },
    uncertain: { on: { SUBMIT: 'pending', ACCEPT: 'accepted', REJECT: 'rejected', RESET: 'idle' } }
  }
});

export function bidStateToPhase(state: string): BidPhase {
  if (state === 'pending' || state === 'confirming') return 'pending';
  if (state === 'confirmRequired') return 'confirm_required';
  if (state === 'accepted') return 'accepted';
  if (state === 'rejected') return 'rejected';
  if (state === 'outbid') return 'rejected';
  if (state === 'uncertain') return 'uncertain';
  return 'idle';
}
