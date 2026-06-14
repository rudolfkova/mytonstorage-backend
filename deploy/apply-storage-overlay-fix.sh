#!/usr/bin/env bash
# Run on vm05 from ~/v3/mytonstorage-backend after git pull.
set -euo pipefail

cd "$(dirname "$0")"

ENV_FILE=".env"
EXTERNAL_IP="${TONUTILS_STORAGE_EXTERNAL_IP:-vm05.proxmox.ip2dns.net}"

# Fix Windows CRLF in .env (breaks docker compose source/parse).
if [ -f "$ENV_FILE" ] && grep -q $'\r' "$ENV_FILE" 2>/dev/null; then
	sed -i 's/\r$//' "$ENV_FILE"
	echo "fixed CRLF in ${ENV_FILE}"
fi

if [ -f "$ENV_FILE" ]; then
	if grep -q '^TONUTILS_STORAGE_EXTERNAL_IP=' "$ENV_FILE"; then
		sed -i "s|^TONUTILS_STORAGE_EXTERNAL_IP=.*|TONUTILS_STORAGE_EXTERNAL_IP=${EXTERNAL_IP}|" "$ENV_FILE"
	else
		printf '\nTONUTILS_STORAGE_EXTERNAL_IP=%s\n' "$EXTERNAL_IP" >>"$ENV_FILE"
	fi
else
	echo "missing ${ENV_FILE}" >&2
	exit 1
fi

docker compose --env-file "$ENV_FILE" -f docker-compose.yml build tonutils-storage
docker compose --env-file "$ENV_FILE" -f docker-compose.yml up -d tonutils-storage

echo "waiting for tonutils-storage..."
sleep 3
docker compose --env-file "$ENV_FILE" -f docker-compose.yml exec tonutils-storage \
	grep -E '"(ExternalIP|ListenAddr)"' /data/db/config.json || \
	docker exec deploy-tonutils-storage-1 grep -E '"(ExternalIP|ListenAddr)"' /data/db/config.json

echo "done — check bag peers via API /api/v1/details"
