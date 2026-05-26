export type AtmosphereKind = 'leading' | 'outbid' | 'extended' | 'sold' | 'recovering' | 'social';
export type AtmosphereUserScope = 'self' | 'other' | 'global';

export type AtmosphereCue = {
  id: number;
  kind: AtmosphereKind;
  title: string;
  detail: string;
  auction_id: string;
  cause_seq: number;
  event_type: string;
  user_scope: AtmosphereUserScope;
  priority: number;
};

export type AtmosphereInput = {
  kind: AtmosphereKind;
  title: string;
  detail: string;
  auction_id: string;
  cause_seq?: number;
  event_type: string;
  user_scope: AtmosphereUserScope;
};

export const atmospherePriority: Record<AtmosphereKind, number> = {
  sold: 100,
  recovering: 90,
  extended: 80,
  outbid: 60,
  leading: 50,
  social: 10
};

export function normalizeAtmosphere(
  input: AtmosphereInput,
  lastSeqValue: number,
  now: () => number = Date.now
): AtmosphereCue | null {
  const causeSeq = input.cause_seq ?? lastSeqValue;
  if (!input.auction_id || !input.event_type || !Number.isFinite(causeSeq) || causeSeq <= 0) return null;
  return {
    id: now(),
    kind: input.kind,
    title: input.title,
    detail: input.detail,
    auction_id: input.auction_id,
    cause_seq: causeSeq,
    event_type: input.event_type,
    user_scope: input.user_scope,
    priority: atmospherePriority[input.kind]
  };
}
