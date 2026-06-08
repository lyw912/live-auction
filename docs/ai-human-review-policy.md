# AI human review policy

This project uses tiered human review so merchants confirm high-impact AI output without approving every low-risk hint.

## Confirm before business change

Use explicit merchant confirmation when AI output can change business state, external buyer-visible listing facts, pricing, rules, winner/order handling, or compliance-sensitive claims.

Current examples:
- Listing draft from image or seller notes: AI creates a draft only. Merchant must confirm adoption into the PC form, then manually create or publish the item.
- Recap rule suggestions for the next item: displayed as advice only. They must not update auction rules automatically.

## Assisted publish, not extra confirmation

When the merchant explicitly clicks a generation action whose result is a short buyer-visible message, treat that click as the intent to publish, but show source and fallback status. Do not add another confirmation step unless the content changes price, rules, item facts, order state, or settlement.

Current examples:
- Manual host talk tracks in the PC console.
- Host-triggered recap/highlight generation.

## Automatic facts and alerts

AI may appear automatically when output is constrained to approved facts, risk alerts, or operational hints. These flows must be easy to turn off or dismiss, must not change business state, and must show a clear boundary when visible to buyers.

Current examples:
- Auto commentary can be toggled per auction.
- Product Q&A answers only from listed item/rule facts and must refuse authenticity, investment, private bid, or unsupported claims.
- Sentinel/risk alerts are advisory and require operator action for any remediation.

References:
- Microsoft Guidelines for Human-AI Interaction: keep users in control and support efficient dismissal/correction.
- Google People + AI Guidebook: calibrate trust with explanations and account for situational stakes.
- NIST AI RMF: define governance and human oversight according to risk and downstream impact.
