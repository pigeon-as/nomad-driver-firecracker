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
	if [ ! -f /tmp/firecracker-images/vmlinux ] || ! file /tmp/firecracker-images/vmlinux | grep -q ELF; then \
		curl -fSL -o /tmp/firecracker-images/vmlinux \
			https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin; \
	fi
	if [ ! -f /tmp/firecracker-images/rootfs.ext4 ]; then \
		curl -fSL -o /tmp/firecracker-images/rootfs.ext4 \
			https://s3.amazonaws.com/spec.ccfc.min/ci-artifacts/disks/x86_64/ubuntu-18.04.ext4; \
	fi
	@mkdir -p $(PLUGIN_DIR)
	go build -o $(PLUGIN_DIR)/$(PLUGIN_BINARY) .
	nomad agent -dev -plugin-dir=$(PLUGIN_DIR) -config=$(shell pwd)/e2e/agent.hcl

e2e:
	go test -tags=e2e -count=1 -v ./e2e
