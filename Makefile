TEST_IMAGE := botl-test:latest

.PHONY: test test-unit test-integration test-e2e test-entrypoint test-image test-clean lint build image build-postrun

## Build the test container image (Docker-in-Docker)
test-image:
	docker build -f test/Dockerfile -t $(TEST_IMAGE) .

## Run all test suites inside a container
test: test-image
	docker run --rm --privileged $(TEST_IMAGE) all

## Run only unit tests inside a container
test-unit: test-image
	docker run --rm --privileged $(TEST_IMAGE) unit

## Run only integration tests (CLI) inside a container
test-integration: test-image
	docker run --rm --privileged $(TEST_IMAGE) integration

## Run only E2E tests (Docker-in-Docker) inside a container
test-e2e: test-image
	docker run --rm --privileged $(TEST_IMAGE) e2e

## Run only entrypoint shell tests inside a container
test-entrypoint: test-image
	docker run --rm --privileged $(TEST_IMAGE) entrypoint

## Remove the test image
test-clean:
	docker rmi -f $(TEST_IMAGE) 2>/dev/null || true

## Run linter
lint:
	golangci-lint run ./...

## Build the botl binary
build:
	go build -o botl .

## Rebuild the embedded botl-postrun binary (linux/amd64) — commit the result
build-postrun:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o internal/container/dockerctx/botl-postrun ./cmd/botl-postrun

## Build the botl Docker image
image: build-postrun
	go run . build
