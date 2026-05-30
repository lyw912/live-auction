#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

if [ "${ALLOW_HISTORICAL_PTS:-}" != "1" ]; then
  echo "reset-hotspot-pressure-data.sh is historical PG-lane/Redis-guard pressure setup." >&2
  echo "Use current PTS-1B reset from tests/pts/MANIFEST.md instead." >&2
  echo "To run this archived script intentionally, set ALLOW_HISTORICAL_PTS=1." >&2
  exit 2
fi

export SESSION_COUNT="${SESSION_COUNT:-4096}"
export SESSION_CSV="${SESSION_CSV:-archive/data/pts_hotspot_sessions.csv}"
export JMX_PATH="${JMX_PATH:-$ROOT_DIR/tests/pts/archive/historical/live-auction-hotspot-pressure.jmx}"
export P1_LOAD_SEED_TIMEOUT="${P1_LOAD_SEED_TIMEOUT:-5m}"
export P1_LOAD_AUC_LIVE_CAP_PRICE_CENTS="${P1_LOAD_AUC_LIVE_CAP_PRICE_CENTS:-100000000000000}"
export P1_LOAD_AUC_SIDE_CAP_PRICE_CENTS="${P1_LOAD_AUC_SIDE_CAP_PRICE_CENTS:-100000000000000}"
export P1_LOAD_AUCTION_END_MINUTES="${P1_LOAD_AUCTION_END_MINUTES:-60}"

cd "$ROOT_DIR"
bash tests/pts/prepare-cloud-pressure.sh

docker exec live-auction-postgres psql -U live_auction -d live_auction -v ON_ERROR_STOP=1 -c "
update auctions
set status='ACTIVE',
    current_price_cents=start_price_cents,
    current_winner_id=null,
    end_at=now() + interval '60 minutes',
    accepted_bid_count=0,
    seq=0,
    updated_at=now()
where id in ('auc_live','auc_side');
"

echo "PTS-1 hotspot pressure data reset complete"
echo "- JMX: tests/pts/archive/historical/live-auction-hotspot-pressure.jmx"
echo "- CSV: docs/perf/pts/${SESSION_CSV}"
