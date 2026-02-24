# Logging

Guest console output and Firecracker daemon logs are captured separately, making them easy to access and debug.

## Guest Console Logs (stdout)

Guest OS serial console output (`/dev/ttyS0`) flows through the executor to Nomad's logmon pipeline, accessible via standard Nomad log commands. Log rotation, size limits, and log disabling behave exactly like other Nomad drivers.

```bash
nomad alloc logs <alloc>
```

### Guest Configuration Required

Kernel boot args must include `console=ttyS0`:
```hcl
config {
  boot_source {
    boot_args = "console=ttyS0 reboot=k panic=1 pci=off"
  }
}
```

Optionally configure systemd services for console visibility:
```ini
[Service]
StandardOutput=journal+console
```

## Firecracker Daemon Logs

The driver always configures Firecracker's built-in logger via `PUT /logger`. Structured JSON daemon logs are written to a file inside the jailer chroot, cleanly separated from guest console output on stdout.

The optional `log_level` field controls verbosity (defaults to `"Warning"`):

```hcl
config {
  log_level = "Info"  # Error, Warning, Info, or Debug (case-sensitive)
}
```

The log file is located at:
```
<chroot_base>/<exec_file>/<task_id>/root/firecracker.log
```

With the default chroot base (`/srv/jailer`), a typical path looks like:
```
/srv/jailer/firecracker/<alloc_id>-<hash>/root/firecracker.log
```

These logs include:
- Firecracker internal operations
- API requests and responses
- VM lifecycle events
- Error messages and warnings

The log file is cleaned up automatically when the task is destroyed (as part of jailer chroot cleanup).

