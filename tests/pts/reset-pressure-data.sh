#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
SESSION_COUNT="${SESSION_COUNT:-4096}"
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
