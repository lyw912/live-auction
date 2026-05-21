# 10 · Test Gates

## Rule

P0 is not done until every P0 gate below has either:

- automated test passing; or
- documented manual test with evidence path and reason automation is not feasible yet.

No P1 work starts before P0 gates are green.

## Unit Tests

| Test | Invariant |
|---|---|
| rule validation | invalid ranges rejected |
| cap reachability | unreachable cap rejected with suggestions |
| increment grid | valid/invalid amounts classified |
| extension calculation | end_at never decreases |
| deposit calculation | integer only, deposit not absurdly above amount |
| reject priority | same input returns fixed code |
| state transitions | illegal transitions rejected |

## Integration Tests

| Test | Invariant |
|---|---|
| create/schedule/start | rules freeze and active created |
| PATCH vs START race | no mid-edit rules |
| bid accepted | bid/event/outbox/idempotency in one tx |
| bid rejected executable | reject stored and idempotent |
| cap sold | order unique and SOLD event |
| end job no winner | ENDED, no order |
| end job winner | SOLD + order |
| payment double click | one PAID transition |
| cancel active | terminal event and later bids reject |

## Concurrency Gates

| Gate | Setup | Expected |
|---|---|---|
| concurrent-final-second | many users bid near end | seq continuous; one winner/order |
| duplicate-idempotency | same key repeated/timeouts | same response or bounded retry-later |
| cancel-cap-race | cancel and cap bid concurrent | exactly one terminal |
| narrate-race | two narrate-start same room | one success, one conflict |
| active-race | two scheduled start same room | one ACTIVE, other retry/conflict |

## Realtime Gates

| Gate | Setup | Expected |
|---|---|---|
| ws-auth-browser | browser connects with ticket | success; invalid ticket rejected |
| forged-room | user joins foreign room | denied and audited |
| history-recovery | reconnect within history | missed events replayed |
| snapshot-fallback | history gap | snapshot applied and CTA recovers |
| reconnect-storm | many stale clients reconnect | DB rebuild bounded |
| slow-consumer | clients stop reading | slow closed, healthy OK |
| out-of-order-detection | inject seq gap | client recovers via snapshot |

## Outbox Gates

| Gate | Setup | Expected |
|---|---|---|
| kill-after-commit | crash after tx commit before publish | relay publishes after restart |
| outbox-order | many events same auction | publish seq monotonic |
| outbox-poison | event repeatedly fails | DEAD + anomaly + gap notice |
| outbox-head-of-line | lower seq failed, higher ready | higher not published until lower PUBLISHED/DEAD |
| outbox-hot-table | sustained burst | metrics captured; no hidden claim |

## Scheduler Gates

| Gate | Setup | Expected |
|---|---|---|
| scheduler-crash | worker dies with RUNNING job | lease expires and another completes once |
| end-job-vs-extend | bid extends after job scheduled | job reschedules, no early hammer |
| order-expire | unpaid order reaches expire | ORDER_EXPIRED once |
| retry-jitter | many jobs conflict | staggered retry |

## Failure/Degradation Gates

| Gate | Setup | Expected |
|---|---|---|
| Redis down bid limit | Redis unavailable | bid limit fail-open + anomaly |
| Redis down reconnect | Redis unavailable | reconnect/snapshot guarded |
| DB lock timeout | hold auction row lock | `BID_RETRY_LATER`, no duplicate |
| idempotency-timeout | stuck PROCESSING | terminal error + anomaly |
| clock-step-backward | scanner sees rollback | anomaly and scheduler pause |

## Frontend Gates

| Gate | Expected |
|---|---|
| H5 state matrix | all states render and no text overlap |
| pending bid | no optimistic success |
| rejected bid | reason-specific copy |
| recovering | CTA disabled |
| cap sold | winner/loser views correct |
| payment double click | no duplicate paid UI |
| PC rule form | illegal cap blocked and backend also rejects |
| diagnostics | real anomaly shown |
| animation longtask | no unacceptable longtask in test build |

## Load Gates

Load gates do not block P0 correctness unless the project wants to claim performance. They block final materials that include performance numbers.

Required before any number:

- final-second bid burst raw output.
- watcher fanout raw output.
- reconnect storm raw output.
- slow-consumer raw output.
- outbox burst raw output.

## Evidence Storage

Every gate records:

```text
Gate:
Date:
Commit:
Command:
Environment:
Result:
Raw output:
Known limits:
Next action:
```

Use `templates/evidence-record.md`.
