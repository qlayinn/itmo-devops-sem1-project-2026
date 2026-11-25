#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
mkdir -p .build
go mod download
go build -o .build/prices-server .

if [[ ${SKIP_DB_INIT:-0} != 1 ]]; then
  .build/prices-server init-db
fi
