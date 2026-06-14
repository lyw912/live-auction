import { setup } from 'xstate';
import type { ConnectionPhase } from '../../domain';

export const connectionMachine = setup({
  types: {
    context: {} as Record<string, never>,
    events: {} as
      | { type: 'CONNECT' }
      | { type: 'OPEN' }
      | { type: 'DISCONNECT' }
      | { type: 'RECOVER' }
      | { type: 'RECOVERED' }
      | { type: 'DEGRADE' }
  }
}).createMachine({
  id: 'h5Connection',
  context: {},
  initial: 'connecting',
  states: {
    connecting: { on: { OPEN: 'connected', DISCONNECT: 'reconnecting', RECOVER: 'resuming' } },
    connected: { on: { DISCONNECT: 'reconnecting', RECOVER: 'resuming', DEGRADE: 'degraded' } },
    reconnecting: { on: { CONNECT: 'connecting', OPEN: 'connected', RECOVER: 'resuming' } },
    resuming: { on: { RECOVERED: 'connected', DISCONNECT: 'reconnecting', DEGRADE: 'degraded' } },
    degraded: { on: { RECOVER: 'resuming', OPEN: 'connected', DISCONNECT: 'reconnecting' } }
  }
});

export function connectionStateToPhase(state: string): ConnectionPhase {
  if (state === 'connected') return 'connected';
  if (state === 'resuming') return 'recovering';
  if (state === 'connecting' || state === 'reconnecting') return 'connecting';
  return 'disconnected';
}
