PLUGIN_BINARY=nomad-driver-firecracker
export GO111MODULE=on

default: build

.PHONY: clean build test
clean: ## Remove build artifacts
	rm -rf ${PLUGIN_BINARY}

build:
	go build -o ${PLUGIN_BINARY} .

test:
	go test ./...
