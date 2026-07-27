# Dogfood harness

Read-only tooling that checks this provider against a **live** Dokploy server.
Neither script can modify the server: both issue HTTP GET only, and Dokploy
exposes every mutation as POST.

| Script | Purpose |
|---|---|
| `introspect.py` | Enumerate a server: projects, environments, services, domains, and their child resources. Secrets are reported as a length, never printed. |
| `generate_imports.py` | Emit Terraform `import` blocks for every live resource this provider supports. |
| `dry-run.sh` | Build the provider, import the live stack into a throwaway state, have Terraform generate the config, and require a second plan to be empty. |

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
  failure path. The cleanup trap is deliberately disarmed for the duration of
  that loop and re-armed immediately after, so the scratch state — including
  whichever imports already succeeded and `generated.tf` — survives for
  inspection.

This is **not** part of CI — it needs a real server, and CI only ever has a
disposable one.

## Known limitation: database engines never reach a clean PASS today

Verified against a live rig (v0.29.13, 2026-07-27, wave-2 task 8): `dry-run.sh`
currently cannot reach `No changes` for **any** stack containing a
`dokploy_postgres`/`mysql`/`mariadb`/`mongo`/`redis` resource, all five engines
alike. The failure happens at the `terraform plan -generate-config-out`
step, before the provider is ever asked to plan anything:

```
Error: Missing Configuration for Required Attribute
  with dokploy_mysql.<label>, on generated.tf line N:
  Must set a configuration value for the database_password attribute as
  the provider has marked it as required.
```

Root cause: `database_password` is `Required` + `Sensitive` in every
engine's schema — correctly so, since the server genuinely requires a
caller-supplied password and never generates one. Terraform's config
generation refuses to write a value for any `Sensitive` attribute
(`null # sensitive` is emitted instead), and a `null` on a `Required`
attribute is then rejected by Terraform Core itself, ahead of any provider
code running. (mysql/mariadb's `database_root_password` is also `Sensitive`
but is `Optional+Computed`, not `Required` — it gets the same `null #
sensitive` treatment in generated config but does NOT error, since Terraform
allows a null on an Optional+Computed attribute and defers to the provider.)
This is not a provider read-path bug: hand-patching the real
`database_password` value into `generated.tf` and continuing the import +
re-plan by hand converges to `No changes` cleanly for every resource in the
stack, including the new mysql/redis resources (full transcript: wave-2
task-8 report). It is a structural mismatch between `-generate-config-out`
and any schema with a `Required`+`Sensitive` attribute, and it is not new to
the four engines this task added — postgres has shipped this way since wave
1. It was simply never exercised end-to-end before, because this harness had
never been run against a stack containing any database engine until now.

Practical effect: the release gate that requires a passing dogfood run
against a stack with a database engine and a domain (wave-2 task 10) is
**not reachable** with the harness in its current form. Fixing it would mean
changing how this harness handles `Required`+`Sensitive` attributes during
config generation — which is a deliberate design question (it likely means
teaching the harness to fetch and inject real secret values, in tension with
this harness's current guarantee of never touching or exposing secrets), not
a small patch, and is left to whoever picks this up next.
