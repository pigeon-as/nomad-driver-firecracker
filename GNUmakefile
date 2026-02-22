PLUGIN_DIR=/tmp/nomad-plugins
PLUGIN_BINARY=nomad-driver-firecracker
export GO111MODULE=on

default: build

.PHONY: clean build test e2e hack

clean: ## Remove build artifacts
	rm -rf ${PLUGIN_BINARY}

build:
	go build -o ${PLUGIN_BINARY} .

test:
	go test ./...

hack: test
	@mkdir -p /tmp/firecracker-images
	if [ ! -f /tmp/firecracker-images/vmlinux ]; then \
		curl -sL -o /tmp/firecracker-images/vmlinux https://s3.amazonaws.com/spec.ccfc.download/img/hello/vmlinux; \
	fi
	if [ ! -f /tmp/firecracker-images/rootfs.ext4 ]; then \
		curl -sL -o /tmp/firecracker-images/rootfs.ext4 https://s3.amazonaws.com/spec.ccfc.download/img/hello/rootfs.ext4; \
	fi
	@mkdir -p $(PLUGIN_DIR)
	go build -o $(PLUGIN_DIR)/$(PLUGIN_BINARY) .
	nomad agent -dev -plugin-dir=$(PLUGIN_DIR) -config=$(shell pwd)/e2e/agent.hcl

e2e:
	go test -tags=e2e -v ./e2e
