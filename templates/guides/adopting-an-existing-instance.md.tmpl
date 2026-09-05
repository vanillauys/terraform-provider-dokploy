---
page_title: "Adopt an existing Dokploy server"
subcategory: ""
description: |-
  Import a running Dokploy server into the Terraform state without a rebuild.
---

# Adopt an existing Dokploy server

Most users reach this provider with a Dokploy server that already runs. This
guide brings that server under Terraform management through **import**. It
never recreates a service.

## Decide what Terraform owns

Before you import anything, decide for each service whether Terraform or the
Dokploy UI owns it. This is not a style choice.

`dokploy_application` and `dokploy_compose` **own the whole service**. An
apply of either resource rewrites the source, build, and environment
configuration of the service. The next apply therefore replaces each setting
that changed in the Dokploy UI. Manage an application or a compose service in
Terraform or in the UI, not in both.

Since v0.4.0, `dokploy_application` writes only the fields that it models.
`watch_paths`, `build_secrets`, `create_env_file`, `enable_submodules`,
`is_static_spa`, `trigger_type`, `heroku_version`, and `railpack_version` are
all schema attributes. Two reflection tests,
`TestDialectARequestsCarryNoBlindFields` and
`TestSaveRequestsReadEveryFieldFromTheModel`, fail the build when a future
field on these endpoints has no attribute. The rewrite is complete but not
lossy: the resource writes what the schema models, and it blanks nothing
silently.

## Database engines own their data mount

A `dokploy_postgres` (or `dokploy_mysql`, `dokploy_mariadb`, `dokploy_mongo`,
`dokploy_redis`, `dokploy_libsql`) creates a volume mount for its data
directory at create time, and owns it from then on. That mount belongs to the
engine resource. Do not give Terraform a second claim on it:

- **Do not import it** as a [`dokploy_mount`](../resources/mount).
  `generate_imports.py` skips it, but a hand-written `import` block does not.
- **Do not declare one in a new configuration.** The same rule applies to a
  stack that you write from scratch. A `dokploy_mount` on the data directory
  of a database gives Terraform a volume that the engine resource recreates on
  its own. A `terraform destroy` of that mount deletes the data directory of
  the database.

Use `dokploy_mount` on a database engine for *additional* volumes only:
configuration files, seed scripts, or an extra data directory that you manage.
Never use it for the data directory of the engine.

## Enumerate the services on the server

The repository ships a read-only harness in `dogfood/`. `introspect.py` and
`generate_imports.py` send HTTP GET requests only, and Dokploy exposes each
mutation as POST. `dry-run.sh` sends no HTTP requests itself: it drives
`terraform` and calls those two scripts. None of the three scripts can modify
the server.

```bash
export DOKPLOY_ENDPOINT=https://dokploy.example.com
export DOKPLOY_API_KEY=...

./dogfood/introspect.py
```

`introspect.py` lists projects, environments, services, domains, and their
child resources. It reports each secret as a length and never prints the
value.

`generate_imports.py` then writes Terraform `import` blocks for the live
resources that this provider supports. It lists each `dokploy_vault_provider`
as a comment instead of an `import` block: the server redacts the config
block, so you import a vault provider by hand and supply the block for its
type. It also skips one record on purpose: the auto-created
data-volume mount of each database engine (`type == "volume"` and
`volumeName == appName + "-data"`). It marks that mount with a
`# skipped <id>: ...` comment in `imports.tf` instead of a silent omission. An
import of that mount as a `dokploy_mount` gives Terraform a volume that the
engine resource recreates on its own, and a `terraform destroy` of that mount
deletes the data directory of the database.

## The `-generate-config-out` gap, and how the harness works around it

`terraform plan -generate-config-out` cannot produce a configuration that
plans cleanly for a stack with a `dokploy_postgres`, `dokploy_mysql`,
`dokploy_mariadb`, `dokploy_mongo`, `dokploy_redis`, or `dokploy_libsql`
resource. The gap affects all six engines. The command fails with:

```
Error: Missing Configuration for Required Attribute
  with dokploy_mysql.<label>, on generated.tf line N:
  Must set a configuration value for the database_password attribute as
  the provider has marked it as required.
```

`database_password` is `Required` and `Sensitive` in the schema of each
engine, and correctly so: the server requires a password from the caller and
never generates one. The configuration generator of Terraform refuses to write
a value for a `Sensitive` attribute and writes `null # sensitive` instead.
Terraform Core then rejects a `null` on a `Required` attribute before any
provider code runs. The error above is the wording of Terraform Core, not of
this provider.

This is a gap in `-generate-config-out`, not a bug in the read path of the
provider. If you write the real password into `generated.tf` by hand and
continue, the plan converges to `No changes`.

`dogfood/dry-run.sh` does this for you. It tolerates the nonzero exit of that
one command and verifies that `generated.tf` exists. It then runs
`generate_imports.py --patch-sensitive`. That option finds each
`<attr> = null # sensitive` marker by pattern and fills in the real value from
the same read-only `.one` endpoint that the harness already calls.

```bash
DOKPLOY_DOGFOOD=1 ./dogfood/dry-run.sh
```

`dry-run.sh` never runs `terraform apply`. It imports the resources into a
throwaway state, generates the configuration, and requires an empty second
plan. On success, it deletes its scratch directory. On failure, it keeps
`dogfood/scratch/` for inspection.

## Two things that import cannot do

**Provider-only attributes do not survive an import.** `deploy_on_change` and
`deployment_timeout` exist only in Terraform. An import seeds them with their
schema defaults, `true` and `"15m"`. If the configuration of an imported
resource sets a different value, the first plan shows one diff for it. That
diff is expected, and the first apply settles it.

**The Dokploy API cannot delete the default `production` environment.** A
`terraform destroy` of an imported `production` environment therefore fails by
design. Remove it from the state instead:

```bash
terraform state rm dokploy_environment.production
```

## After adoption

Run a plan and make sure that it is empty. A plan that stays empty across
later applies shows that the adoption succeeded.

[Deploy semantics](deploy-semantics) describes what starts a deploy after
Terraform owns a service.
