# Logging

The driver automatically captures two log streams through Nomad's executor:

## Firecracker Daemon Logs (stderr.0)

Structured JSON logs from the Firecracker daemon process, written via `--log-path`:

```bash
nomad alloc logs -stderr <alloc>
```

These logs include:
- Firecracker internal operations
- API requests and responses
- VM lifecycle events
- Error messages and warnings

## Guest Console Logs (stdout.0)

Guest OS serial console output (`/dev/ttyS0`) automatically captured via Firecracker's stdout:

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

For structured application logs beyond console output, configure guest to:
- Send logs to external systems (Syslog, Loki, etc.)
- Expose metrics via HTTP endpoints
- Write to files on mounted volumes
