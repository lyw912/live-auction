#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"

if [ "${ALLOW_HISTORICAL_PTS:-}" != "1" ]; then
  echo "reset-pressure-data.sh is historical and is not the current PTS-1B reset." >&2
  echo "Use: L4B_PROFILE=pts-1b SESSION_COUNT=1000 bash tests/pts/reset-l4b-final-second-pressure.sh" >&2
  echo "To run this archived script intentionally, set ALLOW_HISTORICAL_PTS=1." >&2
  exit 2
fi

SESSION_COUNT="${SESSION_COUNT:-4096}"
export SESSION_CSV="${SESSION_CSV:-archive/data/pts_sessions.csv}"
export JMX_PATH="${JMX_PATH:-$ROOT_DIR/tests/pts/archive/historical/live-auction-core-pressure.jmx}"
DATABASE_URL="${DATABASE_URL:-postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-localhost:9000}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-liveauction}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-liveauction123}"
S3_BUCKET="${S3_BUCKET:-live-auction-items}"
S3_USE_SSL="${S3_USE_SSL:-false}"

cd "$ROOT_DIR"
bash tests/pts/prepare-cloud-pressure.sh

docker exec live-auction-postgres psql -U live_auction -d live_auction -v ON_ERROR_STOP=1 -c "
update auctions
set status='ACTIVE',
    current_price_cents=start_price_cents,
    current_winner_id=null,
    end_at=now() + interval '30 minutes',
    accepted_bid_count=0,
    seq=0,
    updated_at=now()
where id in ('auc_live','auc_side');
"

echo "pressure data reset complete"
