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
connection_options=(-o IdentitiesOnly=yes -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=2)
ssh_options=(-i "$DEPLOY_SSH_KEY" -p "$port" "${connection_options[@]}")
scp_options=(-i "$DEPLOY_SSH_KEY" -P "$port" "${connection_options[@]}")

if [[ -n ${DEPLOY_KNOWN_HOSTS:-} || -n ${DEPLOY_HOST_KEY_ALIAS:-} ]]; then
  : "${DEPLOY_KNOWN_HOSTS:?Set DEPLOY_KNOWN_HOSTS together with DEPLOY_HOST_KEY_ALIAS}"
  : "${DEPLOY_HOST_KEY_ALIAS:?Set DEPLOY_HOST_KEY_ALIAS together with DEPLOY_KNOWN_HOSTS}"
  if [[ ! -f $DEPLOY_KNOWN_HOSTS ]]; then
    echo 'DEPLOY_KNOWN_HOSTS does not exist' >&2
    exit 1
  fi
  host_key_options=(-o StrictHostKeyChecking=yes -o "UserKnownHostsFile=$DEPLOY_KNOWN_HOSTS" -o "HostKeyAlias=$DEPLOY_HOST_KEY_ALIAS")
  ssh_options+=("${host_key_options[@]}")
  scp_options+=("${host_key_options[@]}")
fi

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
