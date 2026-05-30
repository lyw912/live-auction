# L4B Kafka Report Review Index

> Status: historical/current-adjacent report index, 2026-05-31.

The report reviews in this folder are valuable failure and bottleneck evidence,
but none of them is a current `CURRENT_PASS` proof for the PTS-1B target. Current
PTS-1B success requires `docs/current/evidence-policy.md`,
`docs/current/runtime-profiles.md`, and `tests/pts/MANIFEST.md`.

## Report Classification

| Report | Original verdict | Current classification | Use |
|---|---|---|---|
| `4I3CX7UG` | `FAIL / HARNESS_GAP` | `HARNESS_ONLY` + correctness failure | Shows PTS loop/harness mismatch and Kafka offset vs `engine_seq` ordering failure |
| `TR3VX7RG` | `HARNESS_GAP` | `HARNESS_ONLY` | Shows barrier timing produced zero useful business traffic |
| `WT3VX7WG` | `PARTIAL_BUSINESS_EVIDENCE` | `CURRENT_ADJACENT` | Shows Redis/Kafka/PG convergence under partial/truncated PTS evidence |
| `913WX7HG` | `CORRECTNESS_PASS` | `CURRENT_ADJACENT` | Shows correctness under arrival reordering; not accepted-hot-path performance evidence |
| `C33WX7MG` | `FAIL_CLOSED_ENGINE_PAUSE` | `CURRENT_FAILING` | Shows aggressive reconcile pause failure mode |
| `U23XX73G` | `CORRECTNESS_PASS_AND_ACCEPTED_LADDER_PASS` | `CURRENT_ADJACENT` | Useful PTS-1A accepted-ladder evidence with PTS count caveat; not PTS-1B success |
| `6A3YX7NG` | `CORRECTNESS_PASS_FOR_PTS_1B_CONTENTION` | `CURRENT_ADJACENT` | Useful correctness evidence for PTS-1B contention; does not prove p99 <= 50ms under the current UX contract |

## Non-Negotiable Reading Rules

- A `CORRECTNESS_PASS` report is not a performance pass.
- A report with PTS sample-count mismatch cannot be cited for full-run p99.
- A report with no Redis/Kafka/PostgreSQL fault injection cannot prove fault
  readiness.
- A report with second-level pending, dominant retry-later, vague `409`, or
  p99 above 50ms fails the current UX/performance target even if final
  settlement is correct.
- Do not combine one report's latency with another report's correctness.

## Current Pass Gate

Future reports may be promoted only if the evidence pack contains:

- runtime profile/env source showing PTS-1B profile, not `.env.example`;
- a review written with `docs/current/pts-run-review-template.md`;
- current JMX/CSV from `tests/pts/MANIFEST.md`;
- reset, preflight, server evidence, PTS report details, and verifier output;
- full `ENGINE_*`, HTTP, durability, and settlement distributions;
- p99 <= 50ms for user-visible engine decision latency;
- highest valid amount wins and every low reject is justified at decision time;
- explicit fault-injection evidence for any resilience claim.
