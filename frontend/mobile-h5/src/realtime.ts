export function reconnectDelayMS(attempt: number, retryAfterMS = 0) {
  if (retryAfterMS > 0) return Math.min(30_000, Math.max(1_000, retryAfterMS));
  const base = Math.min(30_000, 2_000 * (2 ** Math.max(0, attempt - 1)));
  const jitter = base * (0.5 + Math.random() * 1.5);
  return Math.min(30_000, Math.max(1_000, Math.round(jitter)));
}
