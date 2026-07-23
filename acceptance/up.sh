#!/usr/bin/env bash
# Starts a disposable Dokploy instance inside a privileged docker-in-docker
# sandbox. The workstation's own Docker/swarm state is untouched.
# NEVER point acceptance tests at a real instance (server.vnly.io).
set -euo pipefail

NAME="${DOKPLOY_ACC_CONTAINER:-dokploy-acc}"
PORT="${DOKPLOY_ACC_PORT:-3000}"

docker rm -f "$NAME" >/dev/null 2>&1 || true
# DOCKER_IGNORE_BR_NETFILTER_ERROR: this host's kernel doesn't have the
# br_netfilter module loaded, so /proc/sys/net/bridge/bridge-nf-call-iptables
# doesn't exist inside the nested dockerd either. Without this, the inner
# dockerd refuses to create the swarm overlay/bridge networks Dokploy's
# install needs, and the install hangs forever waiting for services to
# converge. This only relaxes an inter-container-isolation check inside the
# disposable sandbox; it has no effect on the host or any real instance.
docker run -d --privileged --name "$NAME" -p "${PORT}:3000" \
  -e DOCKER_IGNORE_BR_NETFILTER_ERROR=1 \
  docker:27-dind >/dev/null

echo "waiting for inner dockerd..." >&2
for _ in $(seq 1 60); do
  docker exec "$NAME" docker info >/dev/null 2>&1 && break
  sleep 2
done
docker exec "$NAME" docker info >/dev/null 2>&1 || {
  echo "inner dockerd never came up" >&2
  exit 1
}

echo "installing dokploy (this pulls images; first run takes minutes)..." >&2
docker exec "$NAME" sh -c \
  'apk add --no-cache bash curl >/dev/null && curl -sSL https://dokploy.com/install.sh | ADVERTISE_ADDR=127.0.0.1 bash'

echo "waiting for the dokploy ui on :${PORT}..." >&2
for _ in $(seq 1 120); do
  if curl -fsS "http://localhost:${PORT}" >/dev/null 2>&1; then
    echo "dokploy is up: http://localhost:${PORT}" >&2
    exit 0
  fi
  sleep 2
done
echo "dokploy never became reachable on :${PORT}" >&2
exit 1
