# Tests

Test assets are grouped by gate type from `docs/s1-s5/00-overview.md`.

- `integration/`: backend cross-module and database behavior.
- `e2e/`: Playwright browser flows.
- `load/`: k6 scripts and raw output references.
- `chaos/`: Toxiproxy scenarios and degradation checks. Start only when needed
  with `docker compose -f infra/docker-compose.yml -f infra/docker-compose.toxiproxy.yml up -d toxiproxy`.

Any P0 gate that cannot be automated immediately needs a manual evidence record under `docs/evidence/`.
