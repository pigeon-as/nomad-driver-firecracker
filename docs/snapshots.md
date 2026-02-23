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
3. **Next start** — detects `snapshot_on_stop = true` and existing snapshot files, links them into the new jailer chroot, starts Firecracker in API-only mode, loads the snapshot, and resumes the VM
4. **Snapshot failure** — if any step fails, the snapshot is discarded and the next start falls back to cold boot

## Snapshot Storage

Snapshot files (`vmstate` and `memory`) are stored in `<task_dir>/snapshots/`. They persist across task restarts within the same allocation.

### Cross-Allocation Persistence

To preserve snapshots across allocation replacements (e.g. scale-to-zero), use Nomad's built-in `ephemeral_disk` with `sticky` and `migrate`:

```hcl
job "example" {
  group "example" {
    ephemeral_disk {
      sticky  = true
      migrate = true
      size    = 2048   # MB — must fit VM memory + vmstate
    }

    task "vm" {
      driver = "firecracker"

      config {
        snapshot_on_stop = true
        ...
      }
    }
  }
}
```

With this configuration:
- `sticky = true` — Nomad prefers placing replacement allocations on the same node
- `migrate = true` — Nomad copies the alloc directory (including snapshots) to the replacement allocation
- The new allocation finds the existing snapshot and resumes instantly

This uses Nomad's native cross-allocation data migration and works correctly with any `count`.

## Timeout Budget

`snapshotOnStop` receives the full `StopTask` timeout as a context deadline. `StopTask` tracks elapsed time and passes whatever remains to `exec.Shutdown`, so the budget is self-managing — no pre-allocation needed.

## Limitations

- **Same filesystem required** — `chroot_base` and Nomad's `data_dir` must be on the same filesystem. Snapshot files are moved and hard-linked (instant metadata operations, zero data copy). If they are on different mounts, snapshot save will fail with a warning and the next start will fall back to cold boot.
- Without `ephemeral_disk { sticky = true, migrate = true }`, snapshots only persist within the same allocation. Config changes create a new allocation → cold boot.
- **Guest clock drift** — the guest OS clock is frozen at snapshot time. After resume the guest clock will be behind wall time. The guest should run an NTP client (e.g. `chrony`) to resync after resume.
- MMDS metadata is re-pushed after every restore (not persisted in the snapshot).
