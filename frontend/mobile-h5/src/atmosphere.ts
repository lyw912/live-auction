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

export type AtmosphereIntensity = 0 | 1 | 2 | 3;

export type AtmosphereSignals = {
  acceptedBids30s?: number;
  priceVelocityCentsPerMin?: number;
  remainingMS?: number | null;
  extended?: boolean;
};

export type AtmosphereGateInput = {
  recovering?: boolean;
  stale?: boolean;
  disconnected?: boolean;
  reducedMotion?: boolean;
  lowPower?: boolean;
  aiOff?: boolean;
};

export type AtmosphereGate = {
  gated: boolean;
  reasons: Array<keyof AtmosphereGateInput>;
  allowMotion: boolean;
  allowAI: boolean;
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

let cueSequence = 0;

export function nextAtmosphereCueID() {
  cueSequence += 1;
  return cueSequence;
}

export const atmospherePriority: Record<AtmosphereKind, number> = {
  sold: 100,
  recovering: 90,
  extended: 80,
  outbid: 60,
  leading: 50,
  social: 10
};

function clampIntensity(value: number): AtmosphereIntensity {
  if (value >= 3) return 3;
  if (value >= 2) return 2;
  if (value >= 1) return 1;
  return 0;
}

export function calculateAtmosphereIntensity(signals: AtmosphereSignals): AtmosphereIntensity {
  const accepted = Math.max(0, signals.acceptedBids30s ?? 0);
  const velocity = Math.max(0, signals.priceVelocityCentsPerMin ?? 0);
  const remaining = signals.remainingMS;
  let intensity = 0;
  if (accepted >= 10) intensity = Math.max(intensity, 3);
  else if (accepted >= 4) intensity = Math.max(intensity, 2);
  else if (accepted >= 1) intensity = Math.max(intensity, 1);
  if (velocity >= 12000) intensity = Math.max(intensity, 3);
  else if (velocity >= 5000) intensity = Math.max(intensity, 2);
  else if (velocity > 0) intensity = Math.max(intensity, 1);
  if (remaining != null && Number.isFinite(remaining) && remaining > 0) {
    if (remaining <= 3000) intensity = Math.max(intensity, 3);
    else if (remaining <= 5000) intensity = Math.max(intensity, 2);
    else if (remaining <= 10000) intensity = Math.max(intensity, 1);
  }
  if (signals.extended && intensity > 0) intensity += 1;
  return clampIntensity(intensity);
}

export function shouldGateAtmosphere(input: AtmosphereGateInput): AtmosphereGate {
  const reasons = (['recovering', 'stale', 'disconnected', 'reducedMotion', 'lowPower', 'aiOff'] as Array<keyof AtmosphereGateInput>)
    .filter((key) => Boolean(input[key]));
  const hardGated = Boolean(input.recovering || input.stale || input.disconnected);
  return {
    gated: hardGated || Boolean(input.reducedMotion || input.lowPower),
    reasons,
    allowMotion: !hardGated && !input.reducedMotion && !input.lowPower,
    allowAI: !hardGated && !input.aiOff
  };
}

export function normalizeAtmosphere(
  input: AtmosphereInput,
  lastSeqValue: number,
  nextID: () => number = nextAtmosphereCueID
): AtmosphereCue | null {
  const causeSeq = input.cause_seq ?? lastSeqValue;
  if (!input.auction_id || !input.event_type || !Number.isFinite(causeSeq) || causeSeq <= 0) return null;
  return {
    id: nextID(),
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
