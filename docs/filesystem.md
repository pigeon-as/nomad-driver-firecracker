# Filesystem Layout

All files for a task live within the allocation directory:

```
allocDir/
├── alloc/              # Nomad allocation data
├── task/               # Task working directory (guest writable)
├── secrets/            # Secrets provisioned by Nomad
├── jailer/
│   └── <task_id>/       # Jailer instance (ID set to task ID)
│       └── root/        # Jailer chroot
│           ├── firecracker # Firecracker daemon
│           ├── run/
│           │   └── firecracker.socket  # HTTP API socket
│           ├── dev/
│           ├── proc/
│           └── sys/
└── snapshot/           # Snapshot files (SIGSTOP)
    ├── memory.img      # VM memory dump
    └── state.vmstate   # VM hardware state
```

## Jailer
- Runs Firecracker in chroot for security isolation
- Binary: configured in plugin config (default: `jailer`)
- Chroot path: always relative to allocDir (not user-configurable)
- Cleanup: automatic on task destroy

## Drive Files
- Must be accessible to Nomad client
- Relative paths: resolved from allocDir
- Absolute paths: used as-is
- Root device required: must have one `is_root_device = true`

## Network
- Tap interfaces: provisioned by Nomad networking
- Interface configuration: included in initial `vmconfig.json` passed to Firecracker at startup
- No bridge setup in driver: delegated to Nomad
- Guest IP configuration: handled inside the VM (cloud-init, systemd-networkd, or custom init)
