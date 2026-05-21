# Tests

Test assets are grouped by gate type from `docs/design-v2-industrial/10-test-gates.md`.

- `integration/`: backend cross-module and database behavior.
- `e2e/`: Playwright browser flows.
- `load/`: k6 scripts and raw output references.
- `chaos/`: Toxiproxy scenarios and degradation checks.

Any P0 gate that cannot be automated immediately needs a manual evidence record under `docs/evidence/`.
