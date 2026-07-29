---
page_title: "Adopting an existing Dokploy instance"
subcategory: ""
description: |-
  Import a running Dokploy server into Terraform state without recreating anything.
---

# Adopting an existing Dokploy instance

Most people reach this provider with a Dokploy server already running. This
guide brings that server under Terraform management by **importing** it, never
by recreating it.

## Decide what Terraform owns

Before importing anything, decide per service whether Terraform or the Dokploy
UI owns it. This is not a stylistic choice.

`dokploy_application` and `dokploy_compose` **own the whole service**. Applying
either rewrites the service's source, build and environment configuration
wholesale, so anything changed in the Dokploy UI is replaced on the next apply.
Manage a given application or compose service in Terraform or in the UI, not
both.

As of v0.4.0 `dokploy_application` no longer writes any field it does not
model: `watch_paths`, `build_secrets`, `create_env_file`, `enable_submodules`,
`is_static_spa`, `trigger_type`, `heroku_version` and `railpack_version` are
all schema attributes, and a pair of reflection tests
(`TestDialectARequestsCarryNoBlindFields`,
`TestSaveRequestsReadEveryFieldFromTheModel`) fail the build if a future field
is added to one of these endpoints without one. So the rewrite is total but not
lossy: what the resource writes is what the schema models, and nothing is
silently blanked.

## Enumerate what is on the server

The repository ships a read-only harness under `dogfood/`. `introspect.py`
and `generate_imports.py` issue HTTP GET only, and Dokploy exposes every
mutation as POST; `dry-run.sh` issues no HTTP itself - it drives `terraform`
and shells out to those same two scripts - so none of the three can modify
the server.

```bash
export DOKPLOY_ENDPOINT=https://dokploy.example.com
export DOKPLOY_API_KEY=...

./dogfood/introspect.py
```

`introspect.py` enumerates projects, environments, services, domains and their
child resources. Secrets are reported as a length, never printed.

`generate_imports.py` then emits Terraform `import` blocks for every live
resource this provider supports, with one deliberate exception: each database
engine's auto-created data-volume mount (`type == "volume"` and `volumeName ==
appName + "-data"`) is skipped, marked with a `# skipped <id>: ...` comment in
`imports.tf` rather than silently omitted. Importing it as a `dokploy_mount`
would hand Terraform a volume the engine resource already recreates on its
own, and `terraform destroy` on that mount would delete the database's data
directory.

## The `-generate-config-out` gap, and why the harness patches around it

`terraform plan -generate-config-out` cannot, by itself, produce a config that
later plans cleanly for **any** stack containing a
`dokploy_postgres`, `dokploy_mysql`, `dokploy_mariadb`, `dokploy_mongo` or
`dokploy_redis` resource. All five engines are affected. You get:

```
Error: Missing Configuration for Required Attribute
  with dokploy_mysql.<label>, on generated.tf line N:
  Must set a configuration value for the database_password attribute as
  the provider has marked it as required.
```

`database_password` is `Required` and `Sensitive` in every engine's schema,
correctly so: the server genuinely requires a caller-supplied password and
never generates one. Terraform's config generation refuses to write a value for
any `Sensitive` attribute, emitting `null # sensitive` instead, and a `null` on
a `Required` attribute is rejected by Terraform Core before any provider code
runs. That error is Terraform Core's own wording, not this provider's.

This is a structural gap in `-generate-config-out`, not a provider read-path
bug: hand-patching the real password into `generated.tf` and continuing
converges to `No changes` cleanly.

`dogfood/dry-run.sh` handles this for you. It tolerates that one command's
nonzero exit, verifies `generated.tf` was still written, then runs
`generate_imports.py --patch-sensitive`, which finds each
`<attr> = null # sensitive` marker by pattern and fills in the real value from
the same read-only `.one` endpoint the harness already calls.

```bash
DOKPLOY_DOGFOOD=1 ./dogfood/dry-run.sh
```

`dry-run.sh` never runs `terraform apply`. It imports into a throwaway state,
generates config, and requires a second plan to be empty. On success it deletes
its scratch directory; on any failure it leaves `dogfood/scratch/` in place for
inspection.

## Two things import cannot do

**Provider-only attributes do not survive import.** `deploy_on_change` and
`deployment_timeout` exist only in Terraform, so import seeds them with their
schema defaults (`true` and `"15m"`). Importing a resource whose configuration
sets a non-default value plans one diff to reconcile it. That diff is expected
and settles on the first apply.

**The default `production` environment cannot be deleted** through Dokploy's
API, so `terraform destroy` on an imported one fails by design. Remove it from
state instead:

```bash
terraform state rm dokploy_environment.production
```

## After adoption

Run a plan and confirm it is empty. A zero-diff plan that stays zero-diff
across subsequent applies is the signal that adoption succeeded.

For what triggers a redeploy once Terraform owns a service, see
[Deploy semantics](deploy-semantics).
