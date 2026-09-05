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

# DOKPLOY_VERSION passes through to install.sh, which reads the same variable:
# a release tag such as v0.30.4 installs that release, an empty or unset value
# installs the latest stable release (the default, and what CI runs). This is
# how a rig for the pinned version or an older release is built.
echo "installing dokploy ${DOKPLOY_VERSION:-latest} (this pulls images; first run takes minutes)..." >&2
docker exec -e DOKPLOY_VERSION="${DOKPLOY_VERSION:-}" "$NAME" sh -c \
  'apk add --no-cache bash curl >/dev/null && curl -sSL https://dokploy.com/install.sh | ADVERTISE_ADDR=127.0.0.1 bash' &
INSTALL_PID=$!

# Some hosts (nested docker-in-docker, e.g. an outer podman runtime without
# br_netfilter) can't route Swarm's VIP/IPVS load-balancing between
# services on a single node. When that happens the "dokploy" app service
# crash-loops forever trying to reach dokploy-postgres by its VIP, and the
# install script's own convergence wait (above) never returns. Switching
# both services to DNS round-robin bypasses IPVS entirely and resolves
# service names straight to real task IPs. This is a harmless no-op on
# hosts where VIP routing already works, so we always apply it, to each
# service as soon as it exists, while the install is still converging.
echo "applying dnsrr endpoint-mode (works around broken swarm VIP routing on some nested-docker hosts; harmless elsewhere)..." >&2
REMEDIATED_PG=0
REMEDIATED_APP=0
for _ in $(seq 1 300); do
  kill -0 "$INSTALL_PID" 2>/dev/null || break
  if [ "$REMEDIATED_PG" -eq 0 ] && docker exec "$NAME" docker service inspect dokploy-postgres >/dev/null 2>&1; then
    docker exec "$NAME" docker service update --endpoint-mode dnsrr dokploy-postgres >/dev/null 2>&1 && REMEDIATED_PG=1
  fi
  if [ "$REMEDIATED_APP" -eq 0 ] && docker exec "$NAME" docker service inspect dokploy >/dev/null 2>&1; then
    docker exec "$NAME" docker service update --endpoint-mode dnsrr dokploy >/dev/null 2>&1 && REMEDIATED_APP=1
  fi
  [ "$REMEDIATED_PG" -eq 1 ] && [ "$REMEDIATED_APP" -eq 1 ] && break
  sleep 2
done

if ! wait "$INSTALL_PID"; then
  echo "dokploy install script failed" >&2
  exit 1
fi

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
