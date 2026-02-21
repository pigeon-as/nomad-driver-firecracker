# Filesystem Layout

All files for a task live within the allocation directory:

```
allocDir/
├── alloc/              # Nomad allocation data
├── task/               # Task working directory (guest writable)
│   └── <task_name>/    # Task instance directory
│       └── jailer/
│           └── <task_id>/   # Jailer instance (ID set to task ID)
│               └── root/    # Jailer chroot (security boundary)
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
└── secrets/            # Secrets provisioned by Nomad
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
3. **Relative Paths**: Once linked, paths are converted to relative filenames (e.g., `kernel` instead of `/full/path/kernel`)
4. **Firecracker Config**: The `vmconfig.json` references files by relative names, which exist inside the chroot
5. **Hard Link Requirements**:
   - Source files must exist and be readable by Nomad client
   - Source and chroot must be on the same filesystem (hard link requirements)
   - Task user (if specified) should own the linked files for VM access

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
