# Filesystem Layout

All files for a task live within the allocation directory:

```
<alloc_dir>/
├── alloc/                  # Nomad shared allocation data
├── <task_name>/            # Task directory (cfg.TaskDir().Dir)
│   └── jailer/
│       └── <exec_file_name>/   # Firecracker binary name (from --exec-file)
│           └── <alloc_id>/     # Jailer instance (ID set to alloc ID)
│               └── root/       # Jailer chroot (security boundary)
│                   ├── firecracker          # Firecracker daemon
│                   ├── vmconfig.json        # VM configuration
│                   ├── kernel               # Kernel image (hard-linked)
│                   ├── initrd               # Initrd if specified (hard-linked)
│                   ├── rootfs.img           # Root drive image (hard-linked)
│                   ├── run/
│                   │   └── firecracker.socket  # HTTP API socket
│                   ├── dev/
│                   ├── proc/
│                   └── sys/
└── secrets/                # Secrets provisioned by Nomad
```

## Jailer
- Runs Firecracker in chroot for security isolation
- Binary: configured in plugin config (default: `jailer`)
- Chroot path: always relative to taskDir (not user-configurable)
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
    # Use absolute paths - driver handles linking into chroot
    kernel_image_path = "/opt/vm-images/kernel"
    boot_args         = "console=ttyS0 root=/dev/vda"
  }
  
  drive {
    # Use absolute paths - driver handles linking into chroot  
    path_on_host   = "/opt/vm-images/rootfs.ext4"
    is_root_device = true
    is_read_only   = false
  }
}
```

## Network
- Tap interfaces: provisioned by Nomad networking
- Interface configuration: included in initial `vmconfig.json` passed to Firecracker at startup
- No bridge setup in driver: delegated to Nomad
- Guest IP configuration: handled inside the VM (cloud-init, systemd-networkd, or custom init)
