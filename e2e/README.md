# E2E Tests

Runs Firecracker VMs via Nomad and checks basic lifecycle.

## Requirements

- Linux with KVM (`/dev/kvm`)
- `firecracker` and `jailer` binaries in `$PATH`
- `nomad` binary in `$PATH`
- [CNI reference plugins](https://developer.hashicorp.com/nomad/docs/deploy#install-cni-reference-plugins) installed at `/opt/cni/bin/` (required for bridge networking tests)
- Root privileges (jailer requirement)
- `docker` (required for `full` test — builds rootfs from OCI image)
- `mkfs.ext4` with `-d` support (e2fsprogs >= 1.43)

### Volume mount tests (additional)

The `TestVolumeMountPersist` test validates the full block device volume mount pipeline: LVM plugin → Nomad volume → Firecracker drive → pigeon-init mount.

Extra requirements:

- **LVM thin pool** on the host. Create one if you don't have it:
  ```sh
  # Example: 1G loopback device (for testing only)
  dd if=/dev/zero of=/tmp/lvm-loop.img bs=1M count=1024
  LOOP=$(losetup --find --show /tmp/lvm-loop.img)
  pvcreate "$LOOP"
  vgcreate vg0 "$LOOP"
  lvcreate -L 900M --thinpool thinpool0 vg0
  ```
- The `make e2e` target automatically builds and installs the LVM plugin via `go install`. Override the VG/pool names with `LVM_PLUGIN_DIR` if your setup differs from `vg0`/`thinpool0`.


## Usage

Start the Nomad dev agent with the plugin:

```sh
make build dev
```

In a second terminal, run everything (downloads kernel/init on first run, builds driver + LVM plugin, then runs tests):

```sh
make e2e
```
