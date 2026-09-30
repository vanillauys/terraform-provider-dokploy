# Contributing

Thank you for your help. This guide tells you how to report a problem, how
to set up the project, and how to send a pull request that can merge.

> [!IMPORTANT]
> **Every commit must be signed.** The `master` branch has the GitHub rule
> [Require signed commits](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches#require-signed-commits).
> GitHub must show **Verified** on each commit of your pull request. A pull
> request with one unsigned commit cannot merge. Set up signing before your
> first commit: see [Sign your commits](#sign-your-commits).

## Contents

- [Report a bug or ask for a feature](#report-a-bug-or-ask-for-a-feature)
- [What you need](#what-you-need)
- [Send a pull request, step by step](#send-a-pull-request-step-by-step)
- [Sign your commits](#sign-your-commits)
- [What CI checks](#what-ci-checks)
- [Build, test, and lint](#build-test-and-lint)
- [Engineering rules](#engineering-rules)

## Report a bug or ask for a feature

- **Bug:** open a
  [bug report](https://github.com/vanillauys/terraform-provider-dokploy/issues/new?template=bug_report.yml).
  Give the provider version, the Dokploy version, the smallest configuration
  that shows the problem, and the error. Remove every secret from the
  output first.
- **Feature:** open a
  [feature request](https://github.com/vanillauys/terraform-provider-dokploy/issues/new?template=feature_request.yml)
  before you write code for a new resource or a new attribute. Name the
  Dokploy endpoint or the UI setting that you want to manage.
- **Vulnerability:** do not open a public issue. Use
  [private vulnerability reporting](https://github.com/vanillauys/terraform-provider-dokploy/security/advisories/new).
  [SECURITY.md](SECURITY.md) gives the scope.

A small fix (a typo, a wrong description, a one-line bug fix) does not need
an issue first.

## What you need

| Tool | Version | For |
|------|---------|-----|
| [Go](https://go.dev/dl/) | the `go` line in [`go.mod`](go.mod) | build and unit tests |
| [Terraform](https://developer.hashicorp.com/terraform/install) | 1.5 or later (1.11 or later for the `_wo` attributes) | acceptance tests |
| [golangci-lint](https://golangci-lint.run/welcome/install/) | the `version:` in [`.github/workflows/test.yml`](.github/workflows/test.yml) | `make lint` |
| [gitleaks](https://github.com/gitleaks/gitleaks#installing) | 8.x | the pre-commit hook |
| [Docker](https://docs.docker.com/engine/install/) | any current release | the local acceptance server |
| GNU Make | any | the targets below |

The unit tests, the linter, and the docs need only Go and golangci-lint. The
acceptance tests need Docker, because they run against a disposable Dokploy
server on your machine.

## Send a pull request, step by step

1. **Fork** the repository on GitHub, then clone your fork:

   ```sh
   git clone git@github.com:<you>/terraform-provider-dokploy.git
   cd terraform-provider-dokploy
   git remote add upstream https://github.com/vanillauys/terraform-provider-dokploy.git
   ```

2. **Set up commit signing** for this clone. Follow
   [Sign your commits](#sign-your-commits), then check it:

   ```sh
   git commit --allow-empty -m "test: signing" && git log --show-signature -1
   git reset --hard HEAD~1
   ```

3. **Enable the git hooks.** The pre-commit hook scans the staged changes
   for secrets with gitleaks:

   ```sh
   make hooks
   ```

4. **Make a branch** from the latest `master`. Use a short name with a
   type prefix, for example `fix/domain-port-revert` or
   `feat/swarm-on-compose`:

   ```sh
   git fetch upstream
   git switch -c fix/<short-name> upstream/master
   ```

5. **Make the change.** Keep one topic in one pull request. For a resource
   or a data source, the
   [pull request template](.github/PULL_REQUEST_TEMPLATE.md) lists every
   file that the change touches. Read
   [Engineering rules](#engineering-rules) before you change a schema or a
   request struct.

6. **Test the change locally:**

   ```sh
   make test   # unit tests and the schema snapshot
   make lint   # must print "0 issues"
   make docs   # regenerate docs/ from the schema, templates/, and examples/
   ```

   For a change to a resource, run its acceptance tests on the local
   server too. See [Acceptance tests](#acceptance-tests).

7. **Add a line to `CHANGELOG.md`** under `## [Unreleased]`, in the
   `Added`, `Changed`, or `Fixed` section. Write what the user sees, and
   the issue number.

8. **Commit** with a [Conventional Commits](https://www.conventionalcommits.org/)
   message: `feat:`, `fix:`, `docs:`, `test:`, `ci:`, or `chore:`. Stage
   the files by name; do not stage files that you did not change.

   ```sh
   git add internal/resources/domain/resource.go CHANGELOG.md
   git commit -m "fix: revert the domain port to null when it is removed (#123)"
   ```

9. **Push** the branch to your fork and **open a pull request** against
   `master`. Fill in the template. Paste the output of the commands that
   you ran into **Verification**.

10. **Wait for the checks.** GitHub holds the workflow runs of a pull
    request from a fork until a maintainer approves them. The checks start
    after that approval. See [What CI checks](#what-ci-checks).

11. **Answer the review.** Push more commits to the same branch. Do not
    force-push after a review starts, so the reviewer can see what changed.
    If `master` moves, merge it into your branch, or let the maintainer
    click **Update branch**.

The maintainer merges with **Squash and merge**. The squashed commit keeps
you as the author.

## Sign your commits

GitHub accepts an SSH signature or a GPG signature. SSH is the shorter
setup if you already push with an SSH key.

### With an SSH key

1. Tell git to sign with your SSH key:

   ```sh
   git config --global gpg.format ssh
   git config --global user.signingkey ~/.ssh/id_ed25519.pub
   git config --global commit.gpgsign true
   ```

2. Add the same public key to GitHub a second time, with the key type
   **Signing Key**: see
   [Adding a new SSH key to your GitHub account](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/adding-a-new-ssh-key-to-your-github-account).
   An **Authentication Key** does not verify commits.

3. To check signatures locally, add your key to an allowed signers file:

   ```sh
   echo "$(git config user.email) $(cat ~/.ssh/id_ed25519.pub)" >> ~/.ssh/allowed_signers
   git config --global gpg.ssh.allowedSignersFile ~/.ssh/allowed_signers
   ```

### With a GPG key

1. [Generate a GPG key](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key)
   and [add it to your GitHub account](https://docs.github.com/en/authentication/managing-commit-signature-verification/adding-a-gpg-key-to-your-github-account).
2. [Tell git about the key](https://docs.github.com/en/authentication/managing-commit-signature-verification/telling-git-about-your-signing-key):

   ```sh
   git config --global user.signingkey <KEY-ID>
   git config --global commit.gpgsign true
   ```

### Check and fix

- The email of the commit must be a verified email on your GitHub account.
  Else GitHub shows **Unverified**.
- `git log --show-signature` shows the signature of each commit locally.
  On GitHub, each commit of the pull request must show **Verified**.
- If a commit of your branch is not signed, sign every commit of the branch
  again and push:

  ```sh
  git rebase --exec 'git commit --amend --no-edit -S' upstream/master
  git push --force-with-lease
  ```

  This is the one case where a force-push to your branch is correct.

GitHub's guide
[About commit signature verification](https://docs.github.com/en/authentication/managing-commit-signature-verification/about-commit-signature-verification)
has the full detail, and
[Troubleshooting commit signature verification](https://docs.github.com/en/authentication/troubleshooting-commit-signature-verification)
explains an **Unverified** badge.

The project has no CLA and needs no DCO `Signed-off-by` line. The signature
proves who made the commit.

## What CI checks

Every pull request runs these checks. The branch rule requires each check
except `gitleaks`, and the branch must be up to date with `master`. A
`gitleaks` failure still stops the merge in review.

| Check | What it does |
|-------|--------------|
| `unit` | `go test -race` on every package, including the schema snapshot |
| `lint` | golangci-lint with the repository configuration |
| `tidy` | `go mod tidy` makes no change |
| `docs` | `make docs` makes no change to `docs/` |
| `vuln` | `govulncheck` finds no reachable vulnerability |
| `acceptance / acceptance` | installs the latest Dokploy in the runner and runs the full acceptance suite |
| `CodeQL` | code scanning of Go, the workflows, and the Python scripts |
| `SonarCloud Code Analysis` | the quality gate, including duplication and coverage of new code |
| `gitleaks` | scans the tree and every commit of the pull request for secrets |

A run from a fork gets no repository secrets, so the SonarQube Cloud job
cannot run on it. When the other checks pass, the maintainer copies your
signed commits to a branch in this repository to run that job. Your
commits and your authorship stay the same.

## Build, test, and lint

The Makefile exports `CGO_ENABLED=0` for each target. Release builds are
already cgo-free, and nothing in the tree needs cgo. On a machine without a C
compiler, a bare `go build ./...` prints a cgo error but still exits 0. If you
call `go` or `golangci-lint` directly on such a machine, set `CGO_ENABLED=0`
yourself. Without it, the linter reports phantom `typecheck` errors that name
different files on each run.

- `make build`: build the provider binary. It also runs `make hooks`.
- `make test`: run the unit tests.
- `make lint`: run golangci-lint. The configuration is in `.golangci.yml`.
  `max-same-issues` is 0, so one run shows every finding.
- `make docs`: regenerate the registry docs, and the Changelog guide from
  `CHANGELOG.md`. Edit `templates/` and `examples/`, never `docs/`. CI fails
  when the regenerated docs differ from the committed docs.
- `go test ./internal/provider -run TestSchemaSnapshot -update`: regenerate
  `internal/provider/testdata/schema.json`, the pinned provider schema. The
  unit tests fail when the schema differs from the snapshot. Review the diff
  of that file in the pull request: a removed attribute, a type change, or an
  optional attribute that became required is a breaking change and needs a
  major version.

### Acceptance tests

The acceptance tests create and delete real records. **Never point them at
a real Dokploy server.** Run them against the disposable server that
`acceptance/up.sh` installs in a Docker container:

```sh
./acceptance/up.sh                     # install a fresh Dokploy (a few minutes)
eval "$(./acceptance/bootstrap.sh)"    # export DOKPLOY_ENDPOINT and DOKPLOY_API_KEY
CGO_ENABLED=0 TF_ACC=1 go test ./internal/resources/domain/... -run TestAcc -v
make testacc                           # the full suite (slow)
```

If the container `dokploy-acc` has only stopped, `docker start dokploy-acc`
restores it in seconds. `up.sh` reinstalls Dokploy from scratch.

### Git hooks

`make hooks` points git at `.githooks/`. The pre-commit hook scans the
staged changes for secrets with
[gitleaks](https://github.com/gitleaks/gitleaks). The allowlist is in
`.gitleaks.toml`. For a confirmed false positive, add a `gitleaks:allow`
comment on the line or extend the allowlist. Do not use `--no-verify`.

### Test layout

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

### A state from the previous release plans no change

A minor release must load a configuration and a state from the previous
release with an empty plan. A new attribute on an existing resource is
therefore `Optional` without a `Default` in most cases: an old state holds
null for it, and a default turns that null into a diff. The upgrade tests
(`TestAcc*_upgradeFrom*`) apply with a released provider version, then plan
with this build, and expect an empty plan.

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
