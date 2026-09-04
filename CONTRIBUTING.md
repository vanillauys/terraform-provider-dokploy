# Contributing

## Build, test, and lint

The Makefile exports `CGO_ENABLED=0` for each target. Release builds are
already cgo-free, and nothing in the tree needs cgo. On a machine without a C
compiler, a bare `go build ./...` prints a cgo error but still exits 0. If you
call `go` or `golangci-lint` directly on such a machine, set `CGO_ENABLED=0`
yourself. Without it, the linter reports phantom `typecheck` errors that name
different files on each run.

- `make test`: run the unit tests.
- `make lint`: run golangci-lint. The configuration is in `.golangci.yml`.
  `max-same-issues` is 0, so one run shows every finding.
- `make docs`: regenerate the registry docs. CI fails when the regenerated
  docs differ from the committed docs.
- `./acceptance/up.sh && eval "$(./acceptance/bootstrap.sh)" && make testacc`:
  run the acceptance tests against a disposable Dokploy server. **Never point
  them at a real server.** If the container `dokploy-acc` has only stopped,
  `docker start dokploy-acc` restores it in seconds. `up.sh` reinstalls
  Dokploy from scratch.

## Git hooks

`make hooks` points git at `.githooks/`. `make build` also runs it. The
pre-commit hook scans the staged changes for secrets with
[gitleaks](https://github.com/gitleaks/gitleaks). The allowlist is in
`.gitleaks.toml`.

## Test layout

Acceptance tests live in `*_acc_test.go` files with `package <pkg>_test`, the
external test package. The `internal/acctest` package imports the provider,
and the provider imports each resource package. An internal test file that
imports `acctest` therefore creates an import cycle. Unit tests stay in the
internal package.

## Engineering rules

Code comments that cite "spec §5.5" or "spec §5.6" refer to the two sections
below. The original design document is not part of the repository.

### §5.5: deploy-engine attributes

Each service resource (application, postgres, and the others) carries two
provider-only attributes from `tfutil.DeployAttributes()`: `deploy_on_change`
(default `true`) and `deployment_timeout` (default `"15m"`). These attributes
exist only in Terraform. `ImportState` must seed their defaults with
`tfutil.ImportDeployDefaults`. Without that step, `terraform import` can never
produce an empty follow-up plan.

### §5.6: optional attributes must revert

When a user removes an optional attribute from the configuration, the
attribute must revert. The acceptance test must prove it with a step that
drops the attribute and asserts `plancheck.ExpectEmptyPlan()`. There are two
shapes, and the difference matters:

- **`Optional` without `Computed` or `Default`** reverts to **null**. The
  server must clear the value. Assert this with a direct API read, not only
  with the Terraform state.
- **`Optional` with `Computed` and a `Default`** reverts to **the default**,
  never to null. Assert the default value, and assert that the server holds
  it.

### Write dialects

Dokploy has three incompatible conventions for an optional field that is
absent from a request. Read the package documentation in
`internal/client/doc.go` before you add or change a request struct. Register
each new request struct in the reflection guard in
`internal/client/dialect_test.go`.

### The sweep rule

A defect on one resource is almost always latent on its sibling resources.
Fix it on **all** of them, and copy the acceptance step that proves the fix.
A fix without the copied test only moves the bug to a quieter place.
