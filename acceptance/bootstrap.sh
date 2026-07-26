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

BODY="$(mktemp)"
trap 'rm -f "$COOKIES" "$BODY"' EXIT

# Readiness is polled, not assumed. A freshly installed instance serves `GET /`
# with 200 before its auth stack will accept a sign-in: nightly run
# 30190067403 got a 403 from the auth endpoints 221ms after the workflow's port
# check passed on its FIRST attempt, while the identical script against the
# identical image digest (dokploy/dokploy:v0.29.13) succeeds locally and on a
# CI re-run. The trigger is intermittent and not observable from outside the
# instance, so rather than waiting on a proxy (a port that answers) we poll the
# condition we actually require — a sign-in that returns 2xx — and on timeout
# report which call failed with which status, so a recurrence diagnoses itself.
READY_TIMEOUT="${DOKPLOY_ACC_READY_TIMEOUT:-180}"
READY_INTERVAL="${DOKPLOY_ACC_READY_INTERVAL:-3}"

# status METHOD URL [curl args...] — prints the HTTP status and writes the body
# to $BODY. Never prints the body: these responses carry session tokens.
status() {
  local method="$1" url="$2"
  shift 2
  curl -sS -o "$BODY" -w '%{http_code}' -X "$method" "$url" "$@" 2>/dev/null || printf '000'
}

# 1+2. Register the admin account and sign in, retrying until the session is
#      real. Sign-up is attempted on every pass and its status deliberately
#      ignored: on a fresh instance it is what creates the account, and on a
#      re-run it returns {"code":"USER_ALREADY_EXISTS_USE_ANOTHER_EMAIL"}
#      forever. Sign-in returning 2xx is the only success condition that
#      distinguishes the two, so that is what is polled.
DEADLINE=$(( $(date +%s) + READY_TIMEOUT ))
SIGNUP_STATUS=000
SIGNIN_STATUS=000
while :; do
  SIGNUP_STATUS="$(status POST "${ENDPOINT}/api/auth/sign-up/email" \
    -c "$COOKIES" -H 'Content-Type: application/json' \
    -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\",\"name\":\"acceptance\"}")"
  SIGNIN_STATUS="$(status POST "${ENDPOINT}/api/auth/sign-in/email" \
    -c "$COOKIES" -b "$COOKIES" -H 'Content-Type: application/json' \
    -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")"
  case "$SIGNIN_STATUS" in 2??) break ;; esac
  if [ "$(date +%s)" -ge "$DEADLINE" ]; then
    echo "bootstrap: the auth stack at ${ENDPOINT} never accepted a sign-in within ${READY_TIMEOUT}s (last sign-up HTTP ${SIGNUP_STATUS}, last sign-in HTTP ${SIGNIN_STATUS}). The instance is serving HTTP but refusing authentication; response bodies are withheld because they carry session tokens." >&2
    exit 1
  fi
  sleep "$READY_INTERVAL"
done

# 3. Resolve the organization id: sign-up auto-creates one ("My
#    Organization") and user.createApiKey requires it explicitly.
ORG_STATUS="$(status GET "${ENDPOINT}/api/trpc/organization.all" -b "$COOKIES")"
case "$ORG_STATUS" in
  2??) ;;
  *) echo "bootstrap: organization.all returned HTTP ${ORG_STATUS} with a session that had just signed in successfully" >&2; exit 1 ;;
esac
ORG_RESPONSE="$(cat "$BODY")"
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
KEY_STATUS="$(status POST "${ENDPOINT}/api/trpc/user.createApiKey" \
  -b "$COOKIES" -H 'Content-Type: application/json' \
  -d "{\"json\":{\"name\":\"${KEY_NAME}\",\"metadata\":{\"organizationId\":\"${ORG_ID}\"},\"rateLimitEnabled\":false}}")"
case "$KEY_STATUS" in
  2??) ;;
  *) echo "bootstrap: user.createApiKey returned HTTP ${KEY_STATUS} (response body withheld, it may contain the real key)" >&2; exit 1 ;;
esac
RESPONSE="$(cat "$BODY")"
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
