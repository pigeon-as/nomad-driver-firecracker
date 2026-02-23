# Filesystem Layout

Files for a task span two directory trees: the Nomad allocation directory and the jailer chroot base.

## Allocation Directory

```
<alloc_dir>/
├── alloc/                  # Nomad shared allocation data
├── <task_name>/            # Task directory (cfg.TaskDir().Dir)
│   └── snapshots/          # Snapshot files (vmstate + memory), persists across restarts
└── secrets/                # Secrets provisioned by Nomad
```

## Jailer Chroot

The jailer chroot lives outside the allocation directory, under `chroot_base` (default `/srv/jailer`):

```
<chroot_base>/                         # e.g. /srv/jailer
└── <exec_file_name>/                  # e.g. firecracker
    └── <taskName>-<allocID>/          # Jailer instance ID
        └── root/                      # Jailer chroot (security boundary)
            ├── firecracker            # Firecracker binary (hard-linked)
            ├── vmconfig.json          # VM configuration
            ├── kernel                 # Kernel image (hard-linked)
            ├── initrd                 # Initrd if specified (hard-linked)
            ├── rootfs.img             # Root drive image (hard-linked)
            ├── run/
            │   └── firecracker.socket # HTTP API socket
            ├── dev/
            ├── proc/
            └── sys/
```

## Jailer
- Runs Firecracker in chroot for security isolation
- Binary: configured in plugin config (default: `jailer`)
- Chroot base: configured via `chroot_base` (default: `/srv/jailer`); must be short to keep the API socket path under the Unix domain socket limit (107 bytes)
- Cleanup: automatic on task destroy
- **File Isolation**: Uses `pivot_root()` to establish security boundary - Firecracker cannot access host paths outside chroot

## Guest Files (Kernel, Initrd, Drives)

The driver automatically handles guest file access via **hard linking**:

1. **Hard Linking Pattern**: Following official Firecracker jailer pattern, the driver hard-links kernel, initrd, and drive images from host paths into the jailer chroot directory during task startup
2. **Why hard links?**: 
   - Provides security: hard links cannot be followed outside the chroot jail
   - More efficient than copies (no space waste)
   - Safer than symlinks (cannot be exploited to escape chroot)
3. **Path Validation**: Before hard linking, paths are validated against allocation directory and optional `image_paths` allowlist
4. **Symlink Resolution**: Symlinks are resolved and re-validated against boundaries before linking
5. **Relative Paths**: Once linked, paths are converted to relative filenames for use in `vmconfig.json`


### Configuration Best Practices

```hcl
config {
  boot_source {
    kernel_image_path = "/opt/vm-images/kernel"
    boot_args         = "console=ttyS0 root=/dev/vda"
  }
  
  drive {
    path_on_host   = "/opt/vm-images/rootfs.ext4"
    is_root_device = true
    is_read_only   = false
  }
}
```

## Network
- **Bridge mode**: When Nomad provides network isolation (bridge/group mode) and no manual network interfaces are configured, the driver creates a TAP device (`tap0`) inside the network namespace with bidirectional TC redirect filters between the veth and TAP. This is idempotent across task restarts.
- **Host mode**: No TAP setup; manual `network_interface` configuration is required
- Interface configuration: included in initial `vmconfig.json` passed to Firecracker at startup
- Guest IP configuration: handled inside the VM (cloud-init, systemd-networkd, or custom init)
