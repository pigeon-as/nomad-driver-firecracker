# Snapshot Boot

Set `snapshot_on_stop = true` in a task to replace cold boots with near-instant VM resume using Firecracker snapshots.

## Usage

```hcl
job "example" {
  group "example" {
    task "vm" {
      driver = "firecracker"

      config {
        snapshot_on_stop = true

        boot_source {
          kernel_image_path = "/opt/vm-images/vmlinux"
          boot_args         = "console=ttyS0 reboot=k panic=1"
        }

        drive {
          path_on_host   = "/opt/vm-images/rootfs.ext4"
          is_root_device = true
        }
      }
    }
  }
}
```

## How It Works

1. **First start** — normal cold boot (no snapshot exists yet)
2. **StopTask** — pauses the VM, creates a snapshot (vmstate + memory), and saves the snapshot files to the snapshot directory
3. **Next start** — detects `snapshot_on_stop = true` and existing snapshot files, links them into the new jailer chroot, starts Firecracker in API-only mode (no `--config-file`), loads the snapshot, and resumes the VM
4. **Snapshot failure** — if any step fails, the snapshot is discarded and the next start falls back to cold boot

## Snapshot Storage

### Ephemeral (default)

When no `snapshot_path` is set in the plugin config, snapshot files are stored in `<task_dir>/snapshots/`. They persist across task restarts within the same allocation but are lost when the allocation is garbage-collected.

### Persistent (scale-to-zero)

When `snapshot_path` is set in the plugin config, snapshot files are stored under `<snapshot_path>/<jobID>/<groupName>/<taskName>/`. This directory is independent of the allocation lifecycle, so snapshots survive allocation GC. This enables scale-to-zero workflows where a job is scaled to 0, then back to 1 — the new allocation finds the existing snapshot and resumes instantly.

```hcl
plugin "firecracker" {
  config {
    snapshot_path = "/opt/vm-snapshots"
    ...
  }
}
```

With this configuration:
- VMs in job `my-job`, group `my-group`, task `my-task` → snapshots at `/opt/vm-snapshots/my-job/my-group/my-task/`
- Each job+group+task combination gets its own isolated directory automatically

## Timeout Budget

`snapshotOnStop` receives the full `StopTask` timeout as a context deadline. `StopTask` tracks elapsed time and passes whatever remains to `exec.Shutdown`, so the budget is self-managing — no pre-allocation needed.

## Limitations

- **Same filesystem required** — `chroot_base` (and `snapshot_path` if set) and Nomad's `data_dir` must be on the same filesystem. Snapshot files are moved and hard-linked (instant metadata operations, zero data copy). If they are on different mounts, snapshot save will fail with a warning and the next start will fall back to cold boot.
- Without `snapshot_path`, snapshots only persist within the same allocation. Config changes create a new allocation → cold boot.
- **Guest clock drift** — the guest OS clock is frozen at snapshot time. After resume the guest clock will be behind wall time. The guest should run an NTP client (e.g. `chrony`) to resync after resume.
- MMDS metadata is re-pushed after every restore (not persisted in the snapshot).
