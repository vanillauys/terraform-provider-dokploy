# Dogfood harness

Read-only tooling that checks this provider against a **live** Dokploy server.
Neither script can modify the server: both issue HTTP GET only, and Dokploy
exposes every mutation as POST.

| Script | Purpose |
|---|---|
| `introspect.py` | Enumerate a server: projects, environments, services, domains, and their child resources. Secrets are reported as a length, never printed. |
| `generate_imports.py` | Emit Terraform `import` blocks for the live resources this provider supports, **except** database data volumes (see below). It does not enumerate `dokploy_compose`, `dokploy_network` or `dokploy_vault_provider` yet. |
| `dry-run.sh` | Build the provider, import the live stack into a throwaway state, have Terraform generate the config (patching in any live secret value Terraform itself refused to write), and require a second plan to be empty. |

## Running it

```bash
export DOKPLOY_ENDPOINT=https://your-dokploy-host
export DOKPLOY_API_KEY=...
DOKPLOY_DOGFOOD=1 ./dogfood/dry-run.sh
```

`dry-run.sh` never runs `terraform apply`, and deletes its scratch directory on
success. On failure it keeps `dogfood/scratch/` so the generated config can be
inspected:

- an empty `imports.tf` (no live resources of any supported type) is a hard
  `FAIL: nothing to import (empty stack or generator bug)` and exits 1 — it
  never falls through to a vacuously empty, and therefore misleadingly
  "passing", plan.
- a failure partway through the `terraform import` loop (for example a stale
  ID from acceptance debris that no longer exists server-side) also preserves
  `dogfood/scratch/` instead of deleting it, the same as the plan-diff
  failure path. Cleanup is a single explicit call made only at the script's
  one PASS exit, never an `EXIT` trap, so no failure path — including one
  partway through this loop — ever deletes `$SCRATCH`; it survives by doing
  nothing, the same as every other failure, so the scratch state (including
  whichever imports already succeeded and `generated.tf`) is left for
  inspection.

This is **not** part of CI — it needs a real server, and CI only ever has a
disposable one.

## Database engines: the Required+Sensitive gap in `-generate-config-out`, and its fix

Discovered against a live rig (v0.29.13, 2026-07-27, wave-2 task 8):
`terraform plan -generate-config-out` cannot, by itself, produce a config
that later plans cleanly for **any** stack containing a
`dokploy_postgres`/`mysql`/`mariadb`/`mongo`/`redis` resource, all five
engines alike (and `dokploy_libsql`, added later with the same schema shape). The command itself exits nonzero (though it still writes
`generated.tf` first — see below):

```
Error: Missing Configuration for Required Attribute
  with dokploy_mysql.<label>, on generated.tf line N:
  Must set a configuration value for the database_password attribute as
  the provider has marked it as required.
```

Root cause: `database_password` is `Required` + `Sensitive` in every
engine's schema — correctly so, since the server genuinely requires a
caller-supplied password and never generates one (see
`internal/client/doc.go:114-121`). Terraform's config generation refuses to
write a value for any `Sensitive` attribute (`null # sensitive` is emitted
instead), and a `null` on a `Required` attribute is then rejected by
Terraform Core itself, ahead of any provider code running — the error above
is Terraform Core's own wording, not the provider's. (mysql/mariadb's
`database_root_password` is also `Sensitive` but is `Optional+Computed`, not
`Required` — it gets the same `null # sensitive` treatment in generated
config but does NOT error, since Terraform allows a null on an
Optional+Computed attribute and defers to the provider.) This is not a
provider read-path bug: hand-patching the real `database_password` value
into `generated.tf` and continuing the import + re-plan by hand converges to
`No changes` cleanly for every resource in the stack (full transcript:
wave-2 task-8 report). It is a structural gap in `-generate-config-out`
alone, and it is not new to the four engines this task added — postgres has
shipped with this exact schema shape since wave 1. It was simply never
exercised end-to-end before, because this harness had never been run
against a stack containing any database engine until this task.

**The fix, now shipped:** `dry-run.sh` tolerates that one command's nonzero
exit (verifying `generated.tf` was still produced), then runs
`generate_imports.py --patch-sensitive` on it before the import loop. That
function walks `generated.tf` for the literal `<attr> = null # sensitive`
pattern Terraform's config generation leaves behind — generically, by
pattern, never by hardcoding an attribute name — and, for each one, reads
the same live, read-only `.one` endpoint this whole harness already calls
to fetch the real value, then writes it into the config in place. The
import loop and the final `-detailed-exitcode` plan run completely
unchanged after that. This strengthens the round-trip assertion rather than
weakening it: with the real password in config, the final `No changes`
result is only obtainable if Read genuinely returns that exact string — a
deliberately wrong value in that same spot reliably produces a reported
diff instead (also verified; see the wave-2 task-8 report's adversarial
re-test).

**This does not introduce a new secrets-exposure surface.** The only
secrets guarantee this repo makes is scoped to `introspect.py`'s *console
output* (this file's table above: "reported as a length, never printed") —
not to `dry-run.sh`, and not to disk. `dry-run.sh` already writes
`$SCRATCH/terraform.tfstate` before this task's changes (Terraform state
stores attribute values, including sensitive ones, in cleartext by design),
already preserves that file on the plan-diff failure path, and this task's
own Step 3 adds a second path that does the same. `.gitignore` already
excludes `dogfood/scratch/` and `*.tfstate` for exactly this reason. The
patched `generated.tf` holds the same secret, in the same directory, for the
same lifecycle (deleted with the rest of `$SCRATCH` on success, preserved
alongside it on failure) as data that was already there — net-new exposure
is zero.

**Live verification scope:** the wave-2 task-8 report's fixture stack
exercises mysql, mariadb, mongo, and redis end to end through this exact
mechanism (all four engines this task added). It does not include a live
`dokploy_postgres` instance — the patch mechanism is written generically
against the resource-type table in `generate_imports.py`, which already
covers `dokploy_postgres`, but that specific engine has not been re-run
through this harness in this task.


## Database engines own a mount nobody asked for

Creating a `dokploy_postgres` — or mysql, mariadb, mongo, redis, libsql — makes
Dokploy attach a volume mount for the container's data directory
immediately. Verified live on the rig (v0.29.13, 2026-07-28): a
freshly-created postgres already owns `volumeName`
`"<appName>-data"` mounted at `/var/lib/postgresql/18/docker`, with nothing
having requested it. A libsql service does the same (verified on v0.30.5,
2026-09-04): `"<appName>-data"` mounted at `/var/lib/sqld`.

It is an ordinary mount — `mounts.remove` deletes it — but it belongs to the
server, not to anyone's configuration. Two things follow:

- **`generate_imports.py` skips it**, and says so with a `# skipped <id>: ...`
  comment in `imports.tf` rather than omitting it silently. Importing it
  would put a `dokploy_mount` in charge of a volume the engine resource
  recreates on its own, and `terraform destroy` on that resource would delete
  the database's data directory.
- The rule is **checked, not guessed**: `type == "volume"` *and* `volumeName
  == appName + "-data"`. A user volume that merely ends in `-data` is still
  imported. `introspect.py` labels the same mounts with the same rule.

## Verified: `--patch-sensitive` covers Optional sensitive attributes too

Wave 2 built `--patch-sensitive` for `database_password`, which is
`Required` + `Sensitive`. Wave 3 added attributes in both shapes and re-ran
the harness against a fixture containing all of them:

| Attribute | Shape | `-generate-config-out` behaviour |
|---|---|---|
| `dokploy_security.password` | Required + Sensitive | `null # sensitive`, then Terraform Core rejects the config |
| `dokploy_destination.access_key` / `secret_access_key` | Required + Sensitive | same |
| `dokploy_application.build_secrets` | **Optional** + Sensitive | `null # sensitive`, accepted by Core, but the plan then diffs against the live value |

The Optional case is the one wave 2 never exercised, and it fails
differently: no error, just a plan that never converges. The patcher matches
on the `<attr> = null # sensitive` pattern rather than on requiredness, so it
handles both — confirmed by a full round-trip over a fixture with a live
`build_secrets` value reaching `No changes` (12 sensitive attributes patched
in that run). An unpatched `build_secrets` would have shown a diff there, so
the empty plan is the proof, not the absence of an error.


## The backup plane has three different discovery paths

Verified live (v0.29.13, 2026-07-28). `generate_imports.py` needs all three,
and reading only the parent record would silently miss two:

| Resource | How it is found |
|---|---|
| `dokploy_backup` | The parent's own embedded `backups` array. There is no `backup.all`, and `backup.create` returns a literal null, so this array is the only place a backup id is ever enumerated. |
| `dokploy_schedule` | `schedule.list`, which requires `id` **and** `scheduleType`. The parent's `schedules` key reads null even when schedules exist. |
| `dokploy_volume_backup` | `volumeBackups.list`, which requires `id` **and** `volumeBackupType`. Its `volumeBackups` key likewise reads null on the parent. |

The two list endpoints validate their type against **different enums**, and
querying one with a type it does not accept is an HTTP 400 rather than an
empty list. Schedules attach to `application`, `compose`, `server` and
`dokploy-server` only — never to a database — while volume backups attach to
any service with a volume, including `redis`. The generator guards on each
enum; discovering the difference as a crash is how this was found.
