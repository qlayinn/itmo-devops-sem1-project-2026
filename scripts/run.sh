#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

if [[ -n ${DEPLOY_HOST:-} ]]; then
  exec "$root/scripts/deploy.sh"
fi

if [[ ! -x .build/prices-server ]]; then
  "$root/scripts/prepare.sh"
fi

RUNNER_TRACKING_ID= nohup "$root/.build/prices-server" </dev/null > "$root/.build/server.log" 2>&1 &
echo $! > "$root/.build/server.pid"

for attempt in {1..30}; do
  if curl --silent --fail "http://localhost:${PORT:-8080}/healthz" > /dev/null; then
    echo "http://localhost:${PORT:-8080}"
    exit 0
  fi
  sleep 1
done

echo 'Server did not become ready. Check .build/server.log' >&2
exit 1
