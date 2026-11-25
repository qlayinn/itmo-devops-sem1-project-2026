#!/usr/bin/env bash
set -euo pipefail

app_user=${1:?Missing application user}

if ! command -v psql > /dev/null; then
  sudo -n apt-get update
  sudo -n apt-get install -y postgresql
fi
sudo -n systemctl enable --now postgresql

if ! sudo -n -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='validator'" | grep -qx 1; then
  sudo -n -u postgres psql -c "CREATE ROLE validator LOGIN"
fi
sudo -n -u postgres psql -c "ALTER ROLE validator WITH PASSWORD 'val1dat0r'"

if ! sudo -n -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='project-sem-1'" | grep -qx 1; then
  sudo -n -u postgres createdb -O validator project-sem-1
fi
sudo -n -u postgres psql -d project-sem-1 -c 'GRANT ALL ON SCHEMA public TO validator'

sudo -n install -m 755 "$HOME/prices-server-linux" /usr/local/bin/prices-server
sudo -n tee /etc/systemd/system/prices-server.service > /dev/null <<UNIT
[Unit]
Description=Prices API
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
User=$app_user
Environment=POSTGRES_HOST=127.0.0.1
Environment=POSTGRES_PORT=5432
Environment=POSTGRES_DB=project-sem-1
Environment=POSTGRES_USER=validator
Environment=POSTGRES_PASSWORD=val1dat0r
ExecStart=/usr/local/bin/prices-server
Restart=on-failure

[Install]
WantedBy=multi-user.target
UNIT

sudo -n systemctl daemon-reload
sudo -n systemctl enable prices-server
sudo -n systemctl restart prices-server

for attempt in {1..30}; do
  if curl --silent --fail http://127.0.0.1:8080/healthz > /dev/null; then
    exit 0
  fi
  sleep 1
done

sudo -n systemctl status prices-server --no-pager
exit 1
