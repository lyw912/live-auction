# Evidence Explanation Template

> Status: governing explanation template, 2026-06-03.
> Purpose: every performance or fault result must be explained at the same
> detail level as the S4 P0 fault explanation. A number without workload,
> timeline, user meaning, and claim boundary is not judge-ready evidence.

## Required Shape

For every run, write the explanation in this order:

1. **Business scenario**
   - What real user/business moment this simulates.
   - Who the users are: bidders, viewers, host, settlement worker, reconnecting
     client, etc.
   - What the user is doing, not just what the script is doing.

2. **Load model**
   - Tool: PTS JMeter, local k6, Docker k6, chaos harness.
   - VU count / connection count / offered RPS.
   - Open model vs closed loop.
   - Per-user pacing: fixed sleep, arrival rate, one-shot, ramp, or hold.
   - Duration and ramp.
   - Dataset/profile: auction id, room id, session count, runtime profile.

3. **Timeline**
   - Warm-up/ramp timing.
   - Fault injection timing if any.
   - Hold/drain/recovery windows.
   - Absolute timestamps when the run has them.

4. **Metrics and boundaries**
   - Define exactly what each headline number measures.
   - State start and stop boundaries, e.g. request start -> final `ENGINE_*`,
     server `published_at_ms` -> client receive, fault clear -> sustained
     recovery, Redis Stream -> Kafka -> PG settlement convergence.
   - State whether the number is user-visible, server-side, or post-run
     convergence.

5. **User-visible interpretation**
   - Translate the metric to what the user sees.
   - Examples: "bid returns paused", "request fails and user retries", "viewer
     sees price update", "payment remains gated while settlement catches up".
   - Distinguish failed request count from failed user count.

6. **Correctness and safety proof**
   - Verifier/gate names and PASS/FAIL.
   - RPO, duplicate settlement, wrong winner, reject justification, pending
     backlog, DLQ, outbox drain, sequence gap, admission contamination.
   - Do not cite latency as success without correctness.

7. **Claim boundary**
   - What this run proves.
   - What it does not prove.
   - Whether it is `CURRENT_PASS`, `CURRENT_FAILING`, `CURRENT_ADJACENT`,
     `HARNESS_ONLY`, or raw/incoming evidence.

8. **Judge wording**
   - One paragraph suitable for the report.
   - One short version suitable for live Q&A.

## Mandatory Tables

### Run Summary

| Field | Value |
|---|---|
| Scenario | S__ / name |
| Run label / report ID | __ |
| Tool | __ |
| Runtime profile | __ |
| Load model | open arrival / closed-loop / WS hold / one-shot |
| Scale | __ VU / __ WS / __ RPS |
| Per-user behavior | __ |
| Duration | __ |
| Fault window | __ |
| Evidence path | __ |

### Result Mapping

| Metric | Raw value | Boundary | User meaning | Verdict |
|---|---:|---|---|---|
| __ | __ | __ | __ | PASS/FAIL |

### Safety Gates

| Gate | Result | What It Proves |
|---|---|---|
| __ | PASS/FAIL | __ |

## S4 P0 Example

Judge-ready wording:

> "S4 P0 uses local k6 with 200 active bidding users in a closed-loop workload.
> Each VU repeatedly reads the auction snapshot, computes a valid bid from the
> current price, posts `/bids`, then sleeps 1s. The run lasts 25s; at T+5s we
> inject a 5s fault. This simulates a live auction where about 200 active bidders
> keep trying to bid while a core dependency fails. Redis SIGKILL produced 800
> `ENGINE_PAUSED` responses overall and zero accepted decisions during the fault
> window, so users saw a safe pause instead of fake success; RTO gate was 2s and
> final convergence from restore start was 19s. Backend/settlement crash produced
> 1200 failed request attempts, not 1200 failed users; after restart Kafka replay
> produced zero duplicate `(epoch, engine_seq)` settlements and zero unsettled
> accepted bids; RTO gate was 2s and final convergence was 17s. PostgreSQL
> failure still allowed 1000 engine decisions during the 5s fault window, proving
> the hot bid path is PG-independent; after PG recovery there were zero
> unsettled accepted bids; RTO gate was 3s and final convergence was 19s. Across
> all three faults, RPO=0 and admission contamination was zero."

Short Q&A wording:

> "This is a 200-active-bidder, one-bid-attempt-per-second, 25s chaos run with a
> 5s fault at T+5s. Redis down means users see a safe pause, not fake accepted
> bids. Backend/settlement down means users see request failures, but replay has
> zero duplicate settlement. PG down means live decisions continue and settlement
> catches up. RTO gates are 2s/2s/3s; final convergence is 19s/17s/19s; RPO=0."

## Rejection Rules

Do not publish a result if any of these are missing:

- scale and duration;
- user behavior/pacing;
- metric boundary;
- correctness verifier or explicit reason why the scenario has no verifier;
- claim boundary and non-claims;
- evidence path or report ID.

Do not use vague phrases:

| Bad | Replace with |
|---|---|
| "200 users pressure" | "200 closed-loop VUs; each reads snapshot, bids, sleeps 1s" |
| "RTO 19s" | "fault clear -> sustained recovery gate 19s; final convergence 35s" |
| "1200 users failed" | "1200 request attempts failed during backend outage; 200 users were looping" |
| "1000 WS passed" | "1000 WS held for 60s; 301 accepted updates; 276000 receive samples; fanout p99 22ms" |
| "PG doesn't matter" | "PG was unavailable for 5s; hot engine still returned 1000 decisions; settlement caught up after recovery" |
