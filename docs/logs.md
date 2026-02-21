# Logging

Both guest console and Firecracker daemon logs are captured and managed by Nomad's logmon pipeline, making them accessible via standard Nomad log commands. This means log rotation, size limits, and log disabling all behave exactly like other Nomad drivers.

## Firecracker Daemon Logs (stdout)

The driver starts Firecracker without a `--log-path` argument, so structured JSON logs from the Firecracker daemon process are emitted to stdout. These logs are captured by the executor and flow through logmon to the task's stdout stream.

These logs include:
- Firecracker internal operations
- API requests and responses
- VM lifecycle events
- Error messages and warnings

Access logs via:
```bash
nomad alloc logs <alloc>
```

## Guest Console Logs

Guest OS serial console output (`/dev/ttyS0`) is emitted to stdout alongside Firecracker daemon logs. View all output via:

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

