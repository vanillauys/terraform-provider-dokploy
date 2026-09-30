# CLAUDE.md

Guidance for a coding agent that works in this repository. People read
[CONTRIBUTING.md](CONTRIBUTING.md); it holds the full rules, and this file
does not repeat them.

## What this is

The Terraform provider `vanillauys/dokploy` for the self-hosted Dokploy
PaaS. Go, `terraform-plugin-framework`, one package per resource under
`internal/resources/`, one per data source under `internal/datasources/`,
and a hand-written HTTP client in `internal/client/`. The provider targets
the Dokploy version in the README badge; the census in
`internal/client/census_test.go` pins its request schema.

## Commands

| Task | Command |
|------|---------|
| Build | `make build` |
| Unit tests and the schema snapshot | `make test` |
| Lint (must print `0 issues`) | `make lint` |
| Regenerate `docs/` | `make docs` |
| Regenerate the schema snapshot | `go test ./internal/provider -run TestSchemaSnapshot -update` |
| Start the local Dokploy server | `./acceptance/up.sh` (fresh) or `docker start dokploy-acc` (stopped) |
| Acceptance credentials | `eval "$(./acceptance/bootstrap.sh)"` |
| Acceptance tests of one package | `CGO_ENABLED=0 TF_ACC=1 go test ./internal/resources/<pkg>/... -run TestAcc -v` |

Set `CGO_ENABLED=0` on every direct `go` or `golangci-lint` command; the
Makefile sets it for its targets. Never point acceptance tests at a real
Dokploy server.

## Rules that break the build or the release

- **Read `internal/client/doc.go` before you touch a request struct.**
  Dokploy has three write dialects for an absent field. Register every new
  request struct in `internal/client/dialect_test.go`.
- **A state from the previous minor release plans no change.** A new
  attribute on an existing resource is `Optional` without a `Default` in
  most cases. The `TestAcc*_upgradeFrom*` tests prove it.
- **Every optional attribute reverts when removed**, and an acceptance step
  proves it with an empty plan (CONTRIBUTING, §5.6).
- **Fix a bug on every sibling resource**, with the copied test.
- **`docs/` is generated.** Edit `templates/`, `examples/`, or the schema
  `Description`, then run `make docs`.
- **Every commit is signed.** The `master` ruleset rejects an unsigned
  commit. Never pass `--no-gpg-sign` or `--no-verify`. Stage files by name.
- **`CHANGELOG.md` is the version record.** Add a line under
  `## [Unreleased]` for each user-visible change.
- The SonarQube Cloud gate fails above 3% duplicated new lines. Put a
  shared schema shape in `internal/tfutil` or `internal/datasources/dsutil`
  rather than a copy per resource.
