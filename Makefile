# Developer commands. Go runs in the pinned golang image (ADR-0006), as the current user.
GO_IMAGE ?= golang:1.24
UID_GID := $(shell id -u):$(shell id -g)
# The module cache is a host directory owned by the current user. A named Docker volume is created root-owned and is
# not writable by a non-root container user, which broke CI (see docs/ci-triage.md).
GO_CACHE_DIR ?= $(HOME)/.cache/plantwin-go
GORUN = mkdir -p $(GO_CACHE_DIR) && docker run --rm --user $(UID_GID) -e HOME=/tmp -e GOPATH=/tmp/go \
        -e GOCACHE=/tmp/gocache -e GOFLAGS=-mod=mod \
        -v $(CURDIR):/src -w /src -v $(GO_CACHE_DIR):/tmp/go $(GO_IMAGE) sh -c

.PHONY: fmt vet lint test golden build image test-ext test-integration demo check

fmt:
	$(GORUN) 'test -z "$$(gofmt -l ./cmd ./internal ./bench)"'

vet:
	$(GORUN) 'go vet ./... && go vet -tags integration ./internal/demo ./internal/export'

# staticcheck is pinned to a release and runs in the golang container, like the rest of the Go tooling (ADR-0006).
lint:
	$(GORUN) 'go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...'

test:
	$(GORUN) 'go test -count=1 ./...'

golden:
	$(GORUN) 'go test -count=1 ./internal/rules -run Golden'

build:
	$(GORUN) 'go build -buildvcs=false -o bin/plantwin ./cmd/plantwin'

# Build the twin image locally. This Makefile never pushes images.
image:
	docker build -t plantwin-postgres:18 .

# Extension regression tests (pg_regress) run in the test stage of the Dockerfile.
test-ext:
	docker build --target test -t plantwin-postgres:18-test .

# Integration tests: the test binary is built in the golang container and run on the host, which has Docker.
test-integration: image
	$(GORUN) 'go test -c -tags integration -buildvcs=false -o bin/demo.test ./internal/demo && go test -c -tags integration -buildvcs=false -o bin/export.test ./internal/export'
	./bin/demo.test -test.v -test.run TestDemo
	./bin/export.test -test.v -test.run 'TestRedaction|TestExportRole'

demo: build
	./bin/plantwin demo

check: fmt vet lint test golden
