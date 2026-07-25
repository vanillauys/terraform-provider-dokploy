default: build

build:
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

.PHONY: default build test testacc lint acc-up docs
