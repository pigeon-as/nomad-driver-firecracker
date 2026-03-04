export GO111MODULE := on

PLUGIN_NAME    := nomad-driver-firecracker
TEST_DIR       := /tmp/testdata
KERNEL_VERSION ?= 6.1.155
INIT_VERSION   ?= latest
PLUGIN_DIR     ?= /tmp/nomad-plugins
PLUGIN_VER     ?= latest

.PHONY: build test init kernel rootfs plugin dev e2e clean

build:
	mkdir -p build
	go build -o build/$(PLUGIN_NAME) .

test:
	go test ./...

init:
	mkdir -p $(TEST_DIR)
	CGO_ENABLED=0 GOOS=linux GOBIN=$(TEST_DIR) go install -trimpath -ldflags="-s -w" github.com/pigeon-as/pigeon-init/cmd/init@$(INIT_VERSION)
	cd $(TEST_DIR) && echo init | cpio -o -H newc > initrd.cpio

kernel:
	mkdir -p $(TEST_DIR)
	curl -fSL -o $(TEST_DIR)/vmlinux.tar.gz \
		https://github.com/pigeon-as/pigeon-kernel/releases/download/v$(KERNEL_VERSION)/pigeon-kernel-$(KERNEL_VERSION)-x86_64.tar.gz
	tar -xzf $(TEST_DIR)/vmlinux.tar.gz -C $(TEST_DIR) vmlinux
	rm -f $(TEST_DIR)/vmlinux.tar.gz

rootfs:
	mkdir -p $(TEST_DIR)
	scripts/build-rootfs.sh alpine:3.20 $(TEST_DIR)/rootfs.ext4
	scripts/build-rootfs.sh hashicorp/http-echo $(TEST_DIR)/http-echo.ext4

plugin:
	mkdir -p $(PLUGIN_DIR)
	GOBIN=$(PLUGIN_DIR) go install github.com/pigeon-as/nomad-plugin-lvm/cmd/nomad-plugin-lvm@$(PLUGIN_VER)

dev: build plugin
	nomad agent -dev -plugin-dir=$(abspath build) -config=$(abspath e2e/agent.hcl)

e2e: kernel init rootfs build plugin
	go test -tags=e2e -count=1 -v ./e2e

clean:
	rm -rf build $(TEST_DIR)
