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
  # Never print the raw response: even though org ids aren't secrets, the
  # response also carries account/session details we have no reason to log.
  echo "failed to resolve an organization id: organization.all returned ${#ORG_RESPONSE} bytes (HTTP call succeeded) but no \"organizationId\" field was found; the response shape may have changed" >&2
  exit 1
fi

# 4. Mint a fresh API key via the tRPC user.createApiKey mutation. Named
#    with a timestamp+pid so re-running this script never collides with a
#    key name from a previous bootstrap run.
#
#    rateLimitEnabled:false is load-bearing: Dokploy's api-key plugin
#    (@better-auth/api-key) rate-limits every key server-side by default
#    (confirmed empirically against this rig: a default key 401s after ~5
#    x-api-key requests, logging "Rate limit exceeded" server-side —
#    surfaced to callers as a generic 401 Unauthorized, not 429, because
#    the failure is caught and folded into "no session"). A single
#    resource's acceptance lifecycle (create+read, refresh+update+read,
#    import+read, destroy) already exceeds that budget, so every
#    acceptance test beyond the simplest would flake without this. This
#    field is accepted here because Dokploy's own tRPC handler calls
#    auth.createApiKey server-side (bypassing the plugin's client-request
#    guard that would otherwise reject caller-supplied rate-limit fields).
KEY_NAME="acceptance-$(date +%s)-$$"
RESPONSE="$(curl -fsS -b "$COOKIES" -X POST "${ENDPOINT}/api/trpc/user.createApiKey" \
  -H 'Content-Type: application/json' \
  -d "{\"json\":{\"name\":\"${KEY_NAME}\",\"metadata\":{\"organizationId\":\"${ORG_ID}\"},\"rateLimitEnabled\":false}}")"
API_KEY="$(printf '%s' "$RESPONSE" | grep -oE '"key":"[^"]*"' | head -1 | cut -d'"' -f4)"

if [ -z "$API_KEY" ]; then
  # Never print $RESPONSE: on a real Dokploy install this body is the
  # createApiKey response and contains the actual secret key, even when
  # our "key" field match fails for shape reasons (renamed field, wrapped
  # envelope, etc.) — a fragment could still leak it.
  echo "failed to extract an api key: createApiKey returned ${#RESPONSE} bytes (HTTP call succeeded) but no \"key\" field was found; the response shape may have changed (response body withheld, it may contain the real key)" >&2
  exit 1
fi

# Register the freshly minted key as a secret with the GitHub Actions runner
# before printing it, so it is redacted from every subsequent log line rather
# than sitting in the workflow log in clear text (the CI jobs feed this
# script's output straight into $GITHUB_ENV).
#
# Two details make this the shape it is:
#   - It belongs HERE, not in the workflow. The workflow only ever sees the key
#     after this script has already printed it, and masking is retroactive only
#     for output the runner has not processed yet.
#   - It goes to STDERR, not stdout. The callers redirect this script's stdout
#     (`bootstrap.sh | sed ... >> "$GITHUB_ENV"`, or `eval "$(bootstrap.sh)"`),
#     so a workflow command written to stdout would be captured into the env
#     file or the eval instead of reaching the runner's log processor. Stderr is
#     left attached to the step, and the runner scans it for workflow commands
#     too.
# Guarded on GITHUB_ACTIONS so a local run doesn't print a stray directive.
if [ -n "${GITHUB_ACTIONS:-}" ]; then
  echo "::add-mask::${API_KEY}" >&2
fi

echo "export DOKPLOY_ENDPOINT=${ENDPOINT}"
echo "export DOKPLOY_API_KEY=${API_KEY}"
