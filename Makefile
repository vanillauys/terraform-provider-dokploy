# CGO is off for every target: release builds already compile with
# CGO_ENABLED=0 (.goreleaser.yml), nothing here needs cgo, and a machine
# without a C compiler otherwise gets phantom failures — a bare `go build`
# even prints the cgo error and still exits 0. Direct `go`/`golangci-lint`
# invocations outside make still need the prefix on such machines.
export CGO_ENABLED = 0

default: build

build: hooks
	go build ./...

test:
	go test ./... -count=1

testacc:
	TF_ACC=1 go test ./internal/... -run 'TestAcc' -v -timeout 60m

lint:
	golangci-lint run

acc-up:
	./acceptance/up.sh

docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name dokploy

# Point git at the version-controlled hooks in .githooks/ so every clone gets
# the gitleaks pre-commit secret scan without a manual step. Idempotent; no-op
# outside a git working copy (e.g. CI source archives).
hooks:
	@[ -d .git ] && git config core.hooksPath .githooks || true

.PHONY: default build test testacc lint acc-up docs hooks
