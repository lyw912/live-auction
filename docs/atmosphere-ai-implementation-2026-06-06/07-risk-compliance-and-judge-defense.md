# Risk, Compliance, And Judge Defense

## Dark Pattern Boundary

Live auctions are naturally urgent. The product line is crossed when urgency or social proof is fabricated.

Allowed:

- true countdown to the server auction end;
- true automatic extension events;
- true active bidder/bid counts;
- true rank/gap/leader state;
- clear opt-in sounds and motion.

Not allowed:

- fake viewer counts;
- fake "many people are watching/bidding" claims;
- fake stock or scarcity;
- fake countdown timers unrelated to the auction;
- AI pressure copy that targets fear, shame, age, or vulnerability.

FTC/OECD dark-pattern guidance explicitly calls out false urgency and false social proof as risky patterns. This is why the first implementation task is removing `2333` and other unsupported labels.

## AI Safety Boundary

AI must be explainable in operational terms:

- input facts are listed;
- output schema is validated;
- unsupported claims are rejected or flagged;
- host reviews listing drafts before publish;
- commentary is tied to event seq;
- sentinel scores are alerts until a human or explicit platform rule acts.

Never answer a judge with "the model decided." The defensible answer is "the model drafted/narrated/flagged; the engine or host made the decision."

## Privacy And Data Minimization

Listing Copilot:

- sends product images/notes only;
- does not need bidder data.

Commentator:

- sends masked user identity and public auction facts;
- does not send private max bids, payment details, or risk scores.
- automatic commentary is a post-decision side effect. If it times out, dedupes, or fails, the bid response and settlement path are unchanged.

Sentinel:

- should start with aggregate features;
- if device/IP/session features are used, document collection purpose and avoid exposing them to host-facing text.

## Expected Judge Questions

**Q: How do you prevent the AI from creating a fake auction fact?**
A: AI outputs are schema-validated and tied to server facts. Commentary receives only decided event facts and source seq. The client renders it as system commentary, not bid truth. The bid engine, terminal events, and settlement remain server-authoritative.

**Q: Why is the new countdown not a dark pattern?**
A: It is derived from the real server end time and disabled when stale/recovering. It never creates a false discount or local winner. At zero it shows syncing until a server terminal event arrives.

**Q: What happens if the AI provider is down during a hot auction?**
A: Manual bidding is unaffected. Listing Copilot is pre-live and can fail a draft job. Commentator falls back to deterministic templates or stays silent. Sentinel can continue with deterministic rules.

**Q: Is shill detection blocking buyers automatically?**
A: Not in the initial design. It creates host/platform alerts and recommended actions. Blocking requires a separate policy decision, evidence threshold, and appeal/override path.

**Q: Why build AI Listing Copilot before more animations?**
A: It solves a real seller cold-start problem and maps directly to non-standard auction items. It is low-risk because it runs before publish and requires human review. Animations improve demo feel; the copilot improves merchant workflow and scoring depth.

**Q: How do you prove the atmosphere did not break performance?**
A: Animation is transform/opacity/canvas-rAF only, reduced-motion is honored, particles are capped, 10Hz root render risk is addressed, and UI traces/visual regression are part of the acceptance gate.

## Failure Modes And Fallbacks

| Failure | Required behavior |
|---|---|
| AI provider timeout | Job fails or template fallback; bid path unaffected. |
| Invalid AI JSON | Reject output and show retry/manual entry. |
| Commentary duplicate replay | Deduplicate by `auction_id + source_seq + kind`. |
| Automatic commentary hidden in PC view | Hide auto-generated messages locally; manual host generation remains available. Server-side per-auction auto toggle is still future scope. |
| Reconnect during critical countdown | Suppress tension, show recovering, disable dangerous actions. |
| Local countdown reaches zero | Show syncing, no celebration. |
| Reduced-motion user | No particles/pulsing; factual text remains. |
| Sound blocked | Show muted/blocked state; no repeated permission nagging. |

## Known-Limits Discipline

Each release note or demo script must say which AI features are:

- live provider-backed;
- fake-provider tested only;
- deterministic template fallback;
- disabled by flag;
- future scope.

This prevents the project from looking like it has hidden mocks when the judge clicks deeper.
