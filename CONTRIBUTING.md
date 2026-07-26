# Contributing

## Build, test, lint

The Makefile exports `CGO_ENABLED=0` for every target — release builds are
already cgo-free, nothing in the tree needs cgo, and on a machine without a C
compiler a bare `go build ./...` prints a cgo error **but still exits 0**. If
you invoke `go` or `golangci-lint` directly on such a machine, carry the
`CGO_ENABLED=0` prefix yourself; symptoms of forgetting it are phantom
`typecheck` lint errors that name different files on each run.

- `make test` — unit tests
- `make lint` — golangci-lint (config in `.golangci.yml`; `max-same-issues` is
  0 so one run shows every finding)
- `make docs` — regenerate registry docs (CI fails if the diff isn't committed)
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc` —
  acceptance tests against a disposable Dokploy. **Never point them at a real
  instance.** If container `dokploy-acc` merely exited, `docker start
  dokploy-acc` restores it in seconds; `up.sh` reinstalls from scratch.

## Git hooks

`make hooks` (also run automatically by `make build`) points git at
`.githooks/`, which pre-commit-scans staged changes for secrets with
[gitleaks](https://github.com/gitleaks/gitleaks). Allowlist:
`.gitleaks.toml`.

## Test layout

Acceptance tests live in `*_acc_test.go` with `package <pkg>_test` (external
test package). `internal/acctest` imports the provider, which imports every
resource package — an internal test file importing `acctest` is an import
cycle. Unit tests stay in the internal package.

## Engineering contract

Code comments citing “spec §5.5” / “spec §5.6” resolve to the sections below
(the original design spec is not distributed with the repo).

### §5.5 — deploy-engine attributes

Every service resource (application, postgres, …) carries two provider-only
attributes via `tfutil.DeployAttributes()`: `deploy_on_change` (default
`true`) and `deployment_timeout` (default `"15m"`). Because they exist only in
Terraform, `ImportState` must seed their defaults with
`tfutil.ImportDeployDefaults` — otherwise `terraform import` can never produce
a clean follow-up plan.

### §5.6 — optional attributes must provably revert

Every optional attribute, when removed from config, must revert — and the
acceptance test must prove it with a step that drops the attribute and asserts
`plancheck.ExpectEmptyPlan()`. Two shapes, and the distinction is load-bearing:

- **`Optional` with no `Computed`/`Default`** reverts to **null**, and the
  server must genuinely clear the value — assert via a direct API read, not
  just Terraform state.
- **`Optional + Computed` with a `Default`** reverts to **its default**, never
  to null — assert the default value, and that the server holds it.

### Write dialects

Dokploy has three incompatible conventions for "this optional field is absent
from my request" — see the package documentation in `internal/client/doc.go`
before adding or changing any request struct, and register new request structs
in the reflection guard in `internal/client/dialect_test.go`.

### The sweep rule

A defect found on one resource is almost always latent on its siblings. Fix it
on **all** of them and replicate the acceptance step that proves the fix —
a fix without the copied test just moves the bug somewhere quieter.
