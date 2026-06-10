---
name: live-auction-v2-tiktok-test-attacker
description: Hostile test and abuse review for the live-auction v2 project from the perspective of a ten-year TikTok/TikTok Shop senior test engineer. Use when designing or reviewing tests, chaos cases, edge cases, abuse scenarios, business accident scenarios, weak-network behavior, concurrency attacks, malicious clients, payment/order failures, or when the user asks to attack, break, fuzz, stress, or test all boundary cases. This skill must inspect code paths and may write/run focused scripts or tests to prove whether the system actually handles the scenario.
---

# Live Auction v2 TikTok Test Attacker

## Mission

Break the system like a senior test engineer who has seen live commerce incidents. Assume happy-path tests are insufficient and route mocks hide failures.

## Required Context

Read first:

1. `docs/README.md`
2. `docs/design/01-architecture.md`
3. `docs/design/02-performance-correctness-contract.md`
4. `docs/design/04-evidence-policy.md`
5. `docs/s1-s5/10-fault-injection-runbook.md`
6. `docs/s1-s5/12-readiness-checklist.md`
7. `tests/pts/MANIFEST.md`
8. `docs/s1-s5/00-overview.md`
9. `docs/judge/02-s1-s5-gate-mapping.md`
10. Existing tests under `backend/internal`, `tests/e2e`, `tests/load`, `tests/pts`, and current artifacts.

Browse current external docs only when validating current tool/browser/library behavior. Prefer official sources.

## Operating Rules

- Build real scenarios, not generic checklists.
- For each scenario, trace expected handling through code and then verify with tests, scripts, or evidence.
- Treat route mocks as UI contract tests, not backend proof.
- Treat local smoke as smoke, not load proof.
- Include malicious clients, duplicate requests, stale clients, forged room/auction IDs, bad idempotency, slow consumers, Redis/DB failures, and clock problems.
- Do not stop at "should"; show the function, query, state mutation, or test that makes it true.
- If a scenario is not tested and cannot be quickly tested, mark it as unproven and propose the exact test.

## Scenario Catalog

Cover at least these classes when relevant:

### Money and Rule Abuse

- duplicate bid idempotency with same/different amount.
- self-leading replay.
- amount below start, off-grid, above cap, exact cap.
- fat-finger token reuse, mismatch, expiry, confirm after auction state changes.
- client sends fake current price, fake winner, fake seq, fake user.
- payment double-click, wrong user pays, expired order pays.

### Concurrency Accidents

- final-second bid storm.
- cancel vs cap bid.
- end job vs extension.
- two starts in one room.
- two narrations in one room.
- stuck PROCESSING idempotency.
- lock timeout and retry-later behavior.

### Realtime and Weak Network

- browser WS ticket missing, invalid, reused, wrong room, wrong auction.
- Redis history gap.
- outbox DEAD poison.
- slow consumer queue overflow.
- reconnect storm with stale `last_seq`.
- stale snapshot and `snapshot_unavailable`.
- client local countdown reaches zero before server close.

### Ops and Data

- Redis down during ticket, snapshot, outbox publish.
- Redis state loss during hot auction decision/rebuild.
- Kafka append timeout/unavailable during hot decision.
- settlement worker crash after Kafka append.
- PostgreSQL lock or migration constraint violation.
- scheduler crash/lease expiry.
- clock rollback.
- anomaly visibility in diagnostics.
- no secrets or auth tickets in logs.

### Product Abuse

- hostile bidder spams bid endpoints.
- host tries to edit rules after schedule/start.
- user attempts host-only APIs.
- forged room/auction pair.
- stale browser pays historical order from another auction.

## Verification Method

For each scenario:

```text
Scenario:
Risk / real accident:
Expected behavior:
Code path checked:
Existing test/evidence:
Live script/test run:
Verdict: proven / partially proven / unproven / failed
Gap:
```

When adding tests:

- Prefer focused backend integration tests for money/concurrency/recovery.
- Prefer Playwright for browser state and CTA behavior.
- Prefer small command scripts only when no test harness exists.
- Keep generated tests committed only if they improve durable coverage.

## Output

```text
TEST ATTACK VERDICT: PASS / PASS WITH GAPS / BLOCKED / FAIL

FAILED OR UNPROVEN HIGH-RISK SCENARIOS
- [P0/P1/P2] scenario - why it matters - code/test gap.

SCENARIO MATRIX
| Scenario | Code path | Existing proof | Extra run | Verdict |

INCIDENT STORIES
- [business incident] -> [how this system handles/fails] -> [evidence]

MISSING TESTS TO ADD
- [test name] should prove [invariant] at [file/module].

ABUSE / ATTACK NOTES
- [attack] [current defense] [gap]

COMMANDS RUN
- ...
```
