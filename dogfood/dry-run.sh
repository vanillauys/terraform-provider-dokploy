#!/usr/bin/env bash
# Import dry-run against a LIVE Dokploy server.
#
# Proves the provider can round-trip real infrastructure: it imports live
# resources into a throwaway state, has Terraform generate the config from
# them, and then requires a second plan to report no changes.
#
# It NEVER runs `terraform apply`. The scratch directory is deleted at the end,
# so nothing is adopted and no state is kept.
set -euo pipefail

: "${DOKPLOY_DOGFOOD:?refusing to touch a live server unless DOKPLOY_DOGFOOD=1 is set}"
: "${DOKPLOY_ENDPOINT:?set DOKPLOY_ENDPOINT}"
: "${DOKPLOY_API_KEY:?set DOKPLOY_API_KEY}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRATCH="$REPO_ROOT/dogfood/scratch"
BIN="$SCRATCH/plugins/registry.terraform.io/vanillauys/dokploy/0.0.0-dev/$(go env GOOS)_$(go env GOARCH)"

cleanup() { rm -rf "$SCRATCH"; }
trap cleanup EXIT

rm -rf "$SCRATCH"
mkdir -p "$BIN"

echo "==> building the provider from the working tree"
(cd "$REPO_ROOT" && CGO_ENABLED=0 go build -o "$BIN/terraform-provider-dokploy_v0.0.0-dev")

echo "==> enumerating the live stack"
python3 "$REPO_ROOT/dogfood/introspect.py" | tee "$SCRATCH/introspection.txt"

echo "==> writing import blocks"
python3 "$REPO_ROOT/dogfood/generate_imports.py" > "$SCRATCH/imports.tf"
import_count="$(grep -c '^import' "$SCRATCH/imports.tf" || true)"
echo "    import blocks: $import_count"
if [ "${import_count:-0}" -eq 0 ]; then
  echo
  echo "FAIL: nothing to import (empty stack or generator bug)"
  exit 1
fi

cat > "$SCRATCH/provider.tf" <<EOF
terraform {
  required_providers {
    dokploy = {
      source  = "vanillauys/dokploy"
      version = "0.0.0-dev"
    }
  }
}

provider "dokploy" {}
EOF

cat > "$SCRATCH/.terraformrc" <<EOF
provider_installation {
  filesystem_mirror {
    path    = "$SCRATCH/plugins"
    include = ["vanillauys/dokploy"]
  }
  direct {
    exclude = ["vanillauys/dokploy"]
  }
}
EOF

export TF_CLI_CONFIG_FILE="$SCRATCH/.terraformrc"

echo "==> terraform init"
terraform -chdir="$SCRATCH" init -input=false

echo "==> generating config from live state"
# Terraform Core never writes a value for a Sensitive schema attribute into
# generated config (it emits `null # sensitive` instead), and a null on a
# Required attribute (database_password, on every database engine) makes
# THIS SAME `plan -generate-config-out` command exit nonzero -- but it still
# writes generated.tf first, with every non-sensitive attribute populated
# correctly and every Required+Sensitive one left as `null # sensitive` for
# the next step to patch. So this specific command's exit code is captured
# rather than trusted to `set -e`, and only treated as fatal if generated.tf
# never actually appeared (a different failure than the one this step exists
# to tolerate).
set +e
terraform -chdir="$SCRATCH" plan -generate-config-out=generated.tf -input=false
generate_status=$?
set -e
if [ ! -s "$SCRATCH/generated.tf" ]; then
  echo
  echo "FAIL: terraform did not produce a generated.tf to patch (exit $generate_status)."
  exit 1
fi
if [ "$generate_status" -ne 0 ]; then
  echo "    (plan -generate-config-out exited $generate_status, generated.tf was still written -- patching Required+Sensitive attributes below before continuing)"
fi

# See dogfood/README.md's "Known limitation" section for the full analysis
# of why the failure above happens and why patching real values back in
# (rather than exempting the attribute from this gate) is the fix. This
# patches every `<attr> = null # sensitive` line back in from the same
# read-only API this whole harness already uses, generically (by pattern,
# not by attribute name).
echo "==> patching Required+Sensitive attributes with live values (read-only)"
python3 "$REPO_ROOT/dogfood/generate_imports.py" --patch-sensitive "$SCRATCH/imports.tf" "$SCRATCH/generated.tf"

# Import blocks only ever get materialized into state by `terraform apply` --
# which this harness must never run. Without an actual import, every
# subsequent plan re-proposes the same "N to import" action forever, so
# -detailed-exitcode can never report 0, even for a perfect round-trip.
#
# `terraform import <addr> <id>` (the classic, standalone command -- not
# `apply`) is the read-only alternative: per its own help text, "This command
# will not modify your infrastructure, but it will make network requests to
# inspect parts of your infrastructure relevant to the resource being
# imported." It writes only to the local scratch state, using the resource
# blocks generate-config-out already produced above.
echo "==> importing into local state (terraform import, not apply -- read-only against the server)"
# Disarm the cleanup trap for the duration of the loop: a failed import here
# (e.g. a stale ID from debris that no longer exists server-side) should leave
# $SCRATCH behind for inspection, the same way the plan-diff failure path
# below already preserves it, instead of deleting the only evidence of what
# went wrong.
trap - EXIT
while IFS=$'\t' read -r addr id; do
  echo "    $addr"
  terraform -chdir="$SCRATCH" import -input=false "$addr" "$id" > /dev/null
done < <(awk -F'"' '
  /^ *to = /{addr=$0; sub(/^ *to = /, "", addr)}
  /^ *id = /{print addr "\t" $2}
' "$SCRATCH/imports.tf")
trap cleanup EXIT

echo "==> re-planning; this must report no changes"
set +e
terraform -chdir="$SCRATCH" plan -input=false -detailed-exitcode
status=$?
set -e

if [ "$status" -eq 0 ]; then
  echo
  echo "PASS: the live stack round-trips with an empty plan."
  exit 0
fi
if [ "$status" -eq 2 ]; then
  echo
  echo "FAIL: the second plan is not empty — the provider cannot round-trip"
  echo "      the live stack. The generated config is at $SCRATCH/generated.tf;"
  echo "      copy it out before this script's cleanup removes it."
  # Keep the evidence for inspection.
  trap - EXIT
  exit 1
fi
echo "FAIL: terraform plan errored (exit $status)"
trap - EXIT
exit "$status"
