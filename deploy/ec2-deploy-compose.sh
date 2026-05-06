#!/usr/bin/env bash
# Deploy one Compose service with a pinned image tag, health check, and rollback to last-known-good tag.
# Pattern matches common VPS deploy scripts (immutable tag + curl health); not registry-GC logic.
#
# Usage (on EC2, from repo root or /opt/pramool):
#   export REGISTRY_HOST REGISTRY_USER REGISTRY_PASSWORD
#   ./ec2-deploy-compose.sh <compose_service> <health_url> <new_image_tag>
#
# Example:
#   ./ec2-deploy-compose.sh pramool-core http://127.0.0.1:3001/healthz a1b2c3d

set -euo pipefail

SERVICE="${1:?compose service name}"
HEALTH_URL="${2:?health check URL (e.g. http://127.0.0.1:3001/healthz)}"
NEW_TAG="${3:?new image tag (e.g. short git sha)}"

DEPLOY_PATH="${DEPLOY_PATH:-/opt/pramool}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"
LAST_GOOD_FILE="${DEPLOY_PATH}/.last-good-${SERVICE}"

cd "$DEPLOY_PATH"

PREV=""
if [[ -f "$LAST_GOOD_FILE" ]]; then
  PREV="$(tr -d ' \t\r\n' <"$LAST_GOOD_FILE" || true)"
fi

echo "${REGISTRY_PASSWORD}" | docker login "${REGISTRY_HOST}" -u "${REGISTRY_USER}" --password-stdin >/dev/null

export REGISTRY_HOST
export IMAGE_TAG="${NEW_TAG}"

docker compose -f "${COMPOSE_FILE}" pull "${SERVICE}"
docker compose -f "${COMPOSE_FILE}" up -d "${SERVICE}"

ok=0
for _ in $(seq 1 30); do
  if curl -sfS --max-time 5 "${HEALTH_URL}" >/dev/null; then
    ok=1
    break
  fi
  sleep 2
done

if [[ "${ok}" != 1 ]]; then
  echo "::error::Health check failed for ${SERVICE} (tag ${NEW_TAG})" >&2
  if [[ -n "${PREV}" && "${PREV}" != "${NEW_TAG}" ]]; then
    echo "Attempting rollback to last-good tag: ${PREV}" >&2
    export IMAGE_TAG="${PREV}"
    docker compose -f "${COMPOSE_FILE}" pull "${SERVICE}"
    docker compose -f "${COMPOSE_FILE}" up -d "${SERVICE}"
    sleep 3
    if curl -sfS --max-time 5 "${HEALTH_URL}" >/dev/null; then
      echo "Rollback: service healthy again on ${PREV}" >&2
    else
      echo "::error::Rollback completed but health check still failing" >&2
    fi
  else
    echo "::error::No previous tag to roll back to (set after first successful deploy)" >&2
  fi
  exit 1
fi

printf '%s' "${NEW_TAG}" >"${LAST_GOOD_FILE}"
echo "Deploy OK ${SERVICE} -> ${NEW_TAG}" >&2
