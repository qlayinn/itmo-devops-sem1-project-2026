#!/usr/bin/env bash
set -euo pipefail

: "${DEPLOY_HOST:?Set DEPLOY_HOST to the server IP address}"
: "${DEPLOY_USER:?Set DEPLOY_USER to the SSH username}"
: "${DEPLOY_SSH_KEY:?Set DEPLOY_SSH_KEY to the private key path}"

if [[ ! $DEPLOY_USER =~ ^[a-z_][a-z0-9_-]*$ ]]; then
  echo 'DEPLOY_USER has an invalid value' >&2
  exit 1
fi
if [[ ! -f $DEPLOY_SSH_KEY ]]; then
  echo 'DEPLOY_SSH_KEY does not exist' >&2
  exit 1
fi

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
port=${DEPLOY_PORT:-22}
target="$DEPLOY_USER@$DEPLOY_HOST"
ssh_options=(-i "$DEPLOY_SSH_KEY" -p "$port" -o IdentitiesOnly=yes -o BatchMode=yes)
scp_options=(-i "$DEPLOY_SSH_KEY" -P "$port" -o IdentitiesOnly=yes -o BatchMode=yes)

machine=$(ssh "${ssh_options[@]}" "$target" uname -m)
case "$machine" in
  x86_64) architecture=amd64 ;;
  aarch64|arm64) architecture=arm64 ;;
  *) echo "Unsupported server architecture: $machine" >&2; exit 1 ;;
esac

mkdir -p .build
GOOS=linux GOARCH="$architecture" CGO_ENABLED=0 go build -o .build/prices-server-linux .
scp "${scp_options[@]}" .build/prices-server-linux scripts/remote-setup.sh "$target:~/"
ssh "${ssh_options[@]}" "$target" "bash ~/remote-setup.sh $DEPLOY_USER"
echo "$DEPLOY_HOST"
