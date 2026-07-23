#!/usr/bin/env bash
# Registers the admin account, resolves its (auto-created) organization, and
# mints a fresh API key on a freshly installed disposable Dokploy instance —
# the same HTTP endpoints the web onboarding / Settings > API-CLI page use.
# Prints export lines for the acceptance tests:
#   eval "$(acceptance/bootstrap.sh)"
#
# Endpoint discovery notes (against a live Dokploy v0.29.13 rig; better-auth):
#   - POST /api/auth/sign-up/email and POST /api/auth/sign-in/email are
#     exactly as guessed and confirmed working as-is.
#   - There is no working "generate token" REST endpoint. The tRPC
#     `user.generateToken` procedure is a stub that unconditionally returns
#     the literal string "token" — it is not wired to anything real. The
#     actual Settings > API/CLI flow calls the tRPC `user.createApiKey`
#     mutation, which requires an organizationId. The public x-api-key REST
#     layer (where project.all etc. live) has no key-minting endpoint, by
#     design: minting your first key requires an authenticated session, so
#     it can't be self-referential.
#   - tRPC calls go through the internal Next.js endpoint at
#     /api/trpc/<procedure>, authenticated with the better-auth session
#     cookie (NOT x-api-key) — GET for queries, POST with a plain
#     `{"json": <input>}` body for mutations. This install answers
#     unbatched calls directly (no `?batch=1` envelope needed).
set -euo pipefail

ENDPOINT="${DOKPLOY_ACC_ENDPOINT:-http://localhost:3000}"
EMAIL="${DOKPLOY_ACC_EMAIL:-acc@example.com}"
PASSWORD="${DOKPLOY_ACC_PASSWORD:-acc-Password-1!}"
COOKIES="$(mktemp)"
trap 'rm -f "$COOKIES"' EXIT

# 1. Register the admin account (idempotent: a 4xx here means it exists —
#    confirmed as {"code":"USER_ALREADY_EXISTS_USE_ANOTHER_EMAIL"}).
curl -sS -c "$COOKIES" -X POST "${ENDPOINT}/api/auth/sign-up/email" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\",\"name\":\"acceptance\"}" >/dev/null 2>&1 || true

# 2. Sign in to get a session cookie (works whether step 1 just created the
#    account or it already existed from a prior bootstrap run).
curl -fsS -c "$COOKIES" -b "$COOKIES" -X POST "${ENDPOINT}/api/auth/sign-in/email" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}" >/dev/null

# 3. Resolve the organization id: sign-up auto-creates one ("My
#    Organization") and user.createApiKey requires it explicitly.
ORG_RESPONSE="$(curl -fsS -b "$COOKIES" "${ENDPOINT}/api/trpc/organization.all")"
ORG_ID="$(printf '%s' "$ORG_RESPONSE" | grep -oE '"organizationId":"[^"]*"' | head -1 | cut -d'"' -f4)"
if [ -z "$ORG_ID" ]; then
  echo "failed to resolve an organization id from: $ORG_RESPONSE" >&2
  exit 1
fi

# 4. Mint a fresh API key via the tRPC user.createApiKey mutation. Named
#    with a timestamp+pid so re-running this script never collides with a
#    key name from a previous bootstrap run.
KEY_NAME="acceptance-$(date +%s)-$$"
RESPONSE="$(curl -fsS -b "$COOKIES" -X POST "${ENDPOINT}/api/trpc/user.createApiKey" \
  -H 'Content-Type: application/json' \
  -d "{\"json\":{\"name\":\"${KEY_NAME}\",\"metadata\":{\"organizationId\":\"${ORG_ID}\"}}}")"
API_KEY="$(printf '%s' "$RESPONSE" | grep -oE '"key":"[^"]*"' | head -1 | cut -d'"' -f4)"

if [ -z "$API_KEY" ]; then
  echo "failed to extract an api key from: $RESPONSE" >&2
  exit 1
fi

echo "export DOKPLOY_ENDPOINT=${ENDPOINT}"
echo "export DOKPLOY_API_KEY=${API_KEY}"
