# Logging

The driver routes guest console output through Nomad's logmon pipeline. This means log rotation, size limits, and log disabling all behave exactly like other Nomad drivers.

## Firecracker Daemon Logs (file)

The driver starts Firecracker with a `--log-path /firecracker.log` argument. Structured JSON logs from the Firecracker daemon process are written to the `/firecracker.log` file inside the jailer chroot, not to the task's stdout or stderr streams.

These logs include:
- Firecracker internal operations
- API requests and responses
- VM lifecycle events
- Error messages and warnings

Access to `/firecracker.log` depends on how the jailer directory is exposed on the host. To consume these logs, ensure that this file is made available via a mounted volume or other host-level log collection mechanism.

## Guest Console Logs (stdout)

Guest OS serial console output (`/dev/ttyS0`) is emitted to the task's stdout stream:

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

## Additional Observability

For structured application logs beyond console output, configure the guest to:
- Send logs to external systems (Syslog, Loki, etc.)
- Expose metrics via HTTP endpoints
- Write to files on mounted volumes
