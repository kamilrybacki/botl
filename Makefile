TEST_IMAGE := botl-test:latest

.PHONY: test test-unit test-integration test-e2e test-entrypoint test-image test-clean lint build image

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

## Build the botl Docker image
image:
	go run . build
