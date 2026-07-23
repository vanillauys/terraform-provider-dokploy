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

.PHONY: default build test testacc lint acc-up
