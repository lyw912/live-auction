# AI Provider Probe

This document is the gate before provider-backed AI development. The current relay candidate is:

- base URL: `https://api.gptgod.online/v1`
- normalized API URL used by the probe: `https://api.gptgod.online/v1`
- model: `gemini-3.1-flash-image-preview`
- backend implementation mode after current probe: `chat_completions_adapter`

Do not commit API keys. Run the probe with an environment variable:

```bash
API_KEY='replace-manually' pnpm test:ai:probe
```

Optional overrides:

```bash
AI_RELAY_BASE_URL='https://api.gptgod.online/v1' \
AI_RELAY_MODEL='gemini-3.1-flash-image-preview' \
AI_RELAY_TIMEOUT_MS=60000 \
AI_RELAY_IMAGE_URL='https://raw.githubusercontent.com/github/explore/main/topics/javascript/javascript.png' \
AI_RELAY_PROBE_OUT='docs/atmosphere-ai-implementation-2026-06-06/evidence/ai-relay-probe-gptgod-latest.json' \
API_KEY='replace-manually' \
pnpm test:ai:probe
```

## Current Verdict

Latest evidence file:

```text
docs/atmosphere-ai-implementation-2026-06-06/evidence/ai-relay-probe-gptgod-latest.json
```

The current relay is usable for P0 provider-backed AI, but not through backend HTTP `/v1/responses`.

- `GET /v1/models`: passed; the exact configured model is listed.
- `/v1/responses`: failed with provider `not implemented` errors for text, JSON schema, and image inputs.
- `/v1/files`: not available on this relay; upload attempts returned `404 page not found`.
- `/v1/chat/completions` plain text: passed.
- `/v1/chat/completions` with strict `response_format.type = "json_schema"`: passed for structured text and listing JSON.
- `/v1/chat/completions` with HTTPS `image_url` and strict JSON schema: passed for multimodal image understanding.
- loose `json_object`: unreliable; use strict `json_schema`.

Decision: P0 AI can start behind an internal provider boundary using `chat_completions_adapter`, strict JSON schema, request timeouts, persistence, validation, and deterministic fallback. Do not implement P0 backend AI by assuming `/v1/responses` or provider file upload works on this relay.

Implementation note from the probe: this model can spend hundreds of completion tokens as reasoning before producing the final JSON. Structured short-text calls failed with a 256-token output cap but passed after raising the cap. Backend calls that require `json_schema` should use a conservative max-token budget and validate that non-empty JSON content was returned.

## Required Capabilities

Backend capability for this project has to be verified with direct HTTP requests. The probe verifies Responses first, but also records Chat Completions compatibility because the current relay exposes the working path through `/v1/chat/completions`.

The final implementation decision must use the latest probe report, not provider assumptions.

The probe requires:

- the exact configured model name appears in `/v1/models`;
- structured text generation with strict JSON schema output through at least one supported API path;
- at least one multimodal input path:
  - preferred if supported by a future probe: upload image through provider Files API and reference it with Responses API `input_image.file_id`;
  - preferred if supported by a future probe: pass a Responses API `input_image.image_url` value;
  - accepted for current P0: pass an HTTPS object URL through Chat Completions `image_url` with `json_schema`.

It also tries, but does not require:

- `GET /v1/models`;
- Chat Completions compatibility checks;
- plain text completion;
- loose JSON output via `response_format: {"type":"json_object"}`;
- data URL image input.
- `GET /v1/files`/upload-style provider file handling if HTTPS image URLs already pass.

If all strict JSON schema paths fail, provider-backed P0 AI must not start. Listing Copilot and AI Commentator both need validated structured output so the backend can reject malformed or unsafe generations.

If Chat Completions plain text or `json_object` fails but strict `json_schema` passes, P0 AI can still proceed by forcing every generation through `json_schema`.

If data URL image input fails but provider file upload or HTTPS image input passes, Listing Copilot can proceed. Inline base64 is not required for production and is not recommended.

For this project, the product flow should still be upload-based:

1. Host uploads images to backend.
2. Backend stores them in object storage and records metadata.
3. Backend issues a short-lived HTTPS object URL from a provider-fetchable domain.
4. AI Listing Copilot receives only the short-lived object URL plus seller notes.
5. The generated listing remains a draft and must be reviewed by the host.

This means merchants see an upload UI, not an HTTP-image field. The model still needs bytes somehow; with the current relay, the reliable model-side transport is a short-lived HTTPS object URL. Do not depend on localhost, intranet URLs, third-party hotlinked fixtures, or manually pasted public image links in production. Provider-native file upload can be added only after the relay passes a file upload probe.

## Decision Rule

- `PASS`: strict JSON schema text and at least one multimodal path passed. P0 provider-backed AI work can start with the `development_gate.wire_api` mode reported by the probe.
- `FAIL`: any required check failed. Do not implement provider-backed AI yet; keep fake provider and deterministic templates only.

The latest report is written to:

```text
docs/atmosphere-ai-implementation-2026-06-06/evidence/ai-relay-probe-gptgod-latest.json
```

## Implementation Implications

When the probe passes:

- backend config should accept `AI_RELAY_BASE_URL`, `AI_RELAY_MODEL`, `API_KEY`, and timeout settings;
- current requests should use `/v1/chat/completions` with `response_format.type = "json_schema"` behind an internal provider interface;
- use a generous output-token cap for strict schema calls because this model may consume reasoning tokens before emitting content;
- keep the internal provider interface API-neutral so a future working `/v1/responses` or provider file upload path can replace the adapter without changing product flows;
- multimodal requests should use a short-lived HTTPS object URL from a domain the model provider can fetch;
- do not use public third-party image hosts as production dependencies; the probe URL is only a capability fixture;
- provider calls must stay outside bidding, settlement, payment, and realtime delivery hot paths;
- provider responses must be persisted with provider, model, prompt version, request hash, output JSON, safety labels, and validation status;
- UI must present AI outputs as drafts/commentary, not facts or decisions;
- failed or slow provider calls must degrade to deterministic templates or disabled state.

Sources used for request-shape expectations:

- OpenAI Chat Completions API reference: https://platform.openai.com/docs/api-reference/chat/create
- OpenAI Images and vision guide: https://platform.openai.com/docs/guides/images-vision
- OpenAI Structured Outputs guide: https://platform.openai.com/docs/guides/structured-outputs
