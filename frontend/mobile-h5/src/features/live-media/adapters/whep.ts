import type { MediaAdapter } from './types';

export const whepAdapter: MediaAdapter = {
  protocols: ['whep'],
  canPlay: () => false,
  attach: () => {
    throw new Error('WHEP adapter not implemented in Phase 2');
  }
};
