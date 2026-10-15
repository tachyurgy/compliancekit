#!/usr/bin/env bash
# Build locally, ship a tiny runtime image to the demo host, swap the
# container, and (re)register it with kamal-proxy.
#
#   HOST=root@<demo-host> deploy/deploy-demo.sh
#
# The host keeps its own env file at /root/compliancekit/.env (see
# deploy/demo.env.example); this script never sends secrets.
set -euo pipefail
HOST=${HOST:?set HOST=user@host}
DOMAIN=${DOMAIN:-compliancekit.levelbrook.com}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

echo "==> frontend (demo build)"
(cd "$ROOT/frontend" && VITE_DEMO_MODE=1 VITE_API_BASE_URL= npm run build >/dev/null)
cp -R "$ROOT/frontend/dist" "$STAGE/web"
rm -f "$STAGE"/web/assets/*.map

echo "==> backend (static linux/amd64 binary)"
(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o "$STAGE/compliancekit-api" ./cmd/server)
cp "$ROOT/deploy/Dockerfile" "$STAGE/Dockerfile"

echo "==> ship + build image on $HOST"
tar -C "$STAGE" -czf "$STAGE/release.tgz" Dockerfile compliancekit-api web
scp -q "$STAGE/release.tgz" "$HOST:/root/compliancekit-release.tgz"
ssh "$HOST" DOMAIN="$DOMAIN" 'bash -s' <<'REMOTE'
set -euo pipefail
mkdir -p /root/compliancekit/release
rm -rf /root/compliancekit/release/*
tar -xzf /root/compliancekit-release.tgz -C /root/compliancekit/release
rm /root/compliancekit-release.tgz
test -f /root/compliancekit/.env || { echo "missing /root/compliancekit/.env"; exit 1; }
docker build -q -t compliancekit:latest /root/compliancekit/release
docker rm -f compliancekit >/dev/null 2>&1 || true
docker run -d --name compliancekit --restart unless-stopped --network kamal \
  --memory 160m --env-file /root/compliancekit/.env \
  -v compliancekit_data:/data compliancekit:latest >/dev/null
sleep 3
docker exec kamal-proxy kamal-proxy deploy compliancekit --target compliancekit:8080 \
  --host "$DOMAIN" --tls --health-check-path /healthz --deploy-timeout 60s
docker image prune -f >/dev/null
docker builder prune -f >/dev/null
docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' compliancekit
REMOTE
echo "==> https://$DOMAIN/healthz: $(curl -s --max-time 10 "https://$DOMAIN/healthz" || echo pending)"
