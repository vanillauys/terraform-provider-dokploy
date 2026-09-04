# Dogfood harness

Read-only tooling that checks this provider against a **live** Dokploy
server. No script can modify the server: each script sends HTTP GET requests
only, and Dokploy exposes each mutation as POST.

| Script | Purpose |
|---|---|
| `introspect.py` | List the contents of a server: projects, environments, services, domains, and their child resources. The script reports each secret as a length and never prints the value. |
| `generate_imports.py` | Write Terraform `import` blocks for the live resources that this provider supports, **except** the data volumes of the database engines (see below). The script does not enumerate `dokploy_compose`, `dokploy_network`, or `dokploy_vault_provider` yet. |
| `dry-run.sh` | Build the provider, import the live stack into a throwaway state, let Terraform generate the configuration, patch in each live secret value that Terraform refused to write, and require an empty second plan. |

## Run the harness

```bash
export DOKPLOY_ENDPOINT=https://your-dokploy-host
export DOKPLOY_API_KEY=...
DOKPLOY_DOGFOOD=1 ./dogfood/dry-run.sh
```

`dry-run.sh` never runs `terraform apply`. On success, it deletes its scratch
directory. On failure, it keeps `dogfood/scratch/` so that you can inspect the
generated configuration:

- An empty `imports.tf` (no live resource of a supported type) is a hard
  failure. The script prints
  `FAIL: nothing to import (empty stack or generator bug)` and exits 1. It
  never continues to an empty, and therefore misleading, plan.
- A failure in the `terraform import` loop also keeps `dogfood/scratch/`. A
  stale id from acceptance-test debris that no longer exists on the server is
  one example. The cleanup is one explicit call at the single PASS exit of the
  script, not an `EXIT` trap. No failure path deletes `$SCRATCH`, so the
  scratch state, the imports that succeeded, and `generated.tf` stay on disk
  for inspection.

The harness is **not** part of CI. It needs a real server, and CI only has a
disposable one.

## Database engines: the Required+Sensitive gap in `-generate-config-out`, and its fix

`terraform plan -generate-config-out` cannot, on its own, produce a
configuration that later plans cleanly for a stack with a database engine
resource. This applies to `dokploy_postgres`, `dokploy_mysql`,
`dokploy_mariadb`, `dokploy_mongo`, `dokploy_redis`, and `dokploy_libsql`. The
command exits nonzero, but it still writes `generated.tf` first:

```
Error: Missing Configuration for Required Attribute
  with dokploy_mysql.<label>, on generated.tf line N:
  Must set a configuration value for the database_password attribute as
  the provider has marked it as required.
```

Root cause: `database_password` is `Required` and `Sensitive` in the schema of
each engine, and correctly so. The server requires a password from the caller
and never generates one. The configuration generator of Terraform refuses to
write a value for a `Sensitive` attribute and writes `null # sensitive`
instead. Terraform Core then rejects a `null` on a `Required` attribute before
any provider code runs. The error above is the wording of Terraform Core, not
of this provider. The `database_root_password` of MySQL and MariaDB is also
`Sensitive`, but it is `Optional` and `Computed`. It gets the same
`null # sensitive` treatment, but it does not cause an error, because
Terraform allows a null on such an attribute and defers to the provider.

This is not a bug in the read path of the provider. If you write the real
`database_password` into `generated.tf` by hand and continue the import and
the second plan, each resource in the stack converges to `No changes`. The gap
is in `-generate-config-out` alone.

**The fix:** `dry-run.sh` tolerates the nonzero exit of that one command and
verifies that `generated.tf` exists. It then runs
`generate_imports.py --patch-sensitive` on the file before the import loop.
That function finds each `<attr> = null # sensitive` marker that the generator
leaves behind. It matches the pattern and never a hardcoded attribute name.
For each marker, it reads the real value from the same live, read-only `.one`
endpoint that the harness already calls, and writes the value into the
configuration in place. The import loop and the final `-detailed-exitcode`
plan run unchanged after that.

The patch strengthens the round-trip assertion. With the real password in the
configuration, the final `No changes` result is only possible when Read
returns that exact string. A deliberately wrong value in the same place
produces a reported diff.

**The patch does not add a new exposure of secrets.** The only secrets
guarantee in this repository is the one in the table above: `introspect.py`
never prints a secret value. That guarantee does not cover `dry-run.sh` or
files on disk. `dry-run.sh` already writes `$SCRATCH/terraform.tfstate`, and
the Terraform state stores attribute values in cleartext by design. The script
keeps that file on each failure path. `.gitignore` excludes `dogfood/scratch/`
and `*.tfstate` for this reason. The patched `generated.tf` holds the same
secret, in the same directory, with the same lifecycle as the state file.

**Verification scope:** a fixture stack with mysql, mariadb, mongo, and redis
went through this mechanism end to end. The mechanism is generic over the
resource-type table in `generate_imports.py`, which also covers
`dokploy_postgres` and `dokploy_libsql`. No run has included a live
`dokploy_postgres` or `dokploy_libsql` instance.

## Verified: `--patch-sensitive` also covers optional sensitive attributes

The first version of `--patch-sensitive` targeted `database_password`, which
is `Required` and `Sensitive`. Later attributes have both shapes. A fixture
with all of them went through the harness:

| Attribute | Shape | `-generate-config-out` behavior |
|---|---|---|
| `dokploy_security.password` | Required + Sensitive | `null # sensitive`, then Terraform Core rejects the configuration |
| `dokploy_destination.access_key` / `secret_access_key` | Required + Sensitive | The same |
| `dokploy_application.build_secrets` | **Optional** + Sensitive | `null # sensitive`, accepted by Core, but the plan then shows a diff against the live value |

The optional case fails differently: no error, only a plan that never
converges. The patcher matches the `<attr> = null # sensitive` pattern and not
the required flag, so it handles both shapes. A full round trip over a fixture
with a live `build_secrets` value reached `No changes`, with 12 patched
sensitive attributes in that run. An unpatched `build_secrets` shows a diff
there, so the empty plan is the proof, not the absence of an error.

## Database engines own a mount that nobody asked for

When you create a `dokploy_postgres`, `dokploy_mysql`, `dokploy_mariadb`,
`dokploy_mongo`, `dokploy_redis`, or `dokploy_libsql`, Dokploy attaches a
volume mount for the data directory of the container at once. A live check on
the rig (Dokploy v0.29.13, 2026-07-28) showed that a new postgres already owns
the `volumeName` `"<appName>-data"` at `/var/lib/postgresql/18/docker`, and
nothing requested it. A libsql service does the same (verified on v0.30.5,
2026-09-04): `"<appName>-data"` at `/var/lib/sqld`.

It is an ordinary mount, and `mounts.remove` deletes it. It belongs to the
server, not to a configuration. Two rules follow:

- **`generate_imports.py` skips it**, and marks the skip with a
  `# skipped <id>: ...` comment in `imports.tf` instead of a silent omission.
  An import of that mount puts a `dokploy_mount` in charge of a volume that
  the engine resource recreates on its own, and a `terraform destroy` of that
  resource deletes the data directory of the database.
- **The rule is checked, not guessed**: `type == "volume"` *and*
  `volumeName == appName + "-data"`. The script still imports a user volume
  whose name only ends in `-data`. `introspect.py` labels the same mounts with
  the same rule.

## The backup plane has three discovery paths

A live check (Dokploy v0.29.13, 2026-07-28) showed three discovery paths.
`generate_imports.py` needs all three. A read of the parent record alone
misses two of them.

| Resource | Discovery path |
|---|---|
| `dokploy_backup` | The embedded `backups` array of the parent. There is no `backup.all`, and `backup.create` returns a literal null, so this array is the only place that lists a backup id. |
| `dokploy_schedule` | `schedule.list`, which requires `id` **and** `scheduleType`. The `schedules` key of the parent reads null even when schedules exist. |
| `dokploy_volume_backup` | `volumeBackups.list`, which requires `id` **and** `volumeBackupType`. The `volumeBackups` key of the parent also reads null. |

The two list endpoints validate their type against **different enums**. A
query with a type that the endpoint does not accept returns HTTP 400, not an
empty list. Schedules attach to `application`, `compose`, `server`, and
`dokploy-server` only, never to a database. Volume backups attach to any
service with a volume, Redis included. The generator guards on each enum.
