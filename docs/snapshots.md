# Snapshot Boot

Enable `snapshot_boot` on a task to replace cold boots with near-instant VM resume using Firecracker snapshots.

## Usage

```hcl
job "example" {
  group "example" {
    task "vm" {
      driver = "firecracker"

      config {
        snapshot_boot = true

        boot_source {
          kernel_image_path = "/tmp/firecracker-images/vmlinux"
          boot_args         = "console=ttyS0 reboot=k panic=1"
        }

        drive {
          path_on_host   = "/tmp/firecracker-images/rootfs.ext4"
          is_root_device = true
        }
      }
    }
  }
}
```

## How It Works

1. **First start** — normal cold boot (no snapshot exists yet)
2. **StopTask** — pauses the VM, creates a snapshot (vmstate + memory), and saves the snapshot files to a persistent directory in the Nomad task directory (`<task_dir>/snapshots/`)
3. **Next start** — detects existing snapshot, links snapshot files into the new jailer chroot, starts Firecracker in API-only mode (no `--config-file`), loads the snapshot, and resumes the VM
4. **Snapshot failure** — if any step fails, the snapshot is discarded and the next start falls back to cold boot

## Snapshot Storage

Snapshot files (`vmstate` and `memory`) are stored in `<task_dir>/snapshots/`. This directory lives outside the jailer chroot and survives `DestroyTask` cleanup, so snapshots persist across task restarts within the same allocation.

## Timeout Budget

`snapshotOnStop` receives the full `StopTask` timeout as a context deadline. `StopTask` tracks elapsed time and passes whatever remains to `exec.Shutdown`, so the budget is self-managing — no pre-allocation needed.

## Limitations

- Snapshots only persist within the same allocation. Config changes create a new allocation → cold boot.
- **Guest clock drift** — the guest OS clock is frozen at snapshot time. After resume the guest clock will be behind wall time. The guest should run an NTP client (e.g. `chrony`) to resync after resume.
- MMDS metadata is re-pushed after every restore (not persisted in the snapshot).
- Snapshot files are hard-linked into the chroot on restore. If the task directory and `chroot_base` are on different filesystems, the driver falls back to copying.
