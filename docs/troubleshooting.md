# Troubleshooting

This guide covers common debugging techniques for the Firecracker Nomad driver.

## Checking allocation status

After submitting a job, check its status:

```bash
nomad job run e2e/jobs/basic.hcl
nomad job status basic
```

Look at the **Allocations** table. If the status is `failed`, note the allocation ID prefix (e.g. `41fa4a14`).

## Reading task logs

Nomad captures stdout and stderr from the jailer/firecracker process via logmon. Stdout carries guest console output; stderr carries jailer startup errors. Firecracker daemon logs are written to a separate file inside the jailer chroot (see [logs.md](logs.md) for details).

### Guest console (stdout)

```bash
nomad alloc logs <alloc_id>
```

Or via the filesystem:

```bash
nomad alloc fs <alloc_id> alloc/logs/firecracker.stdout.0
```

> **Tip:** The guest kernel must boot with `console=ttyS0` for serial output to appear here.

### Stderr

```bash
nomad alloc logs -stderr <alloc_id>
```

Stderr contains jailer startup errors (e.g. invalid instance ID, missing exec-file). Firecracker daemon messages do **not** appear here — they go to the daemon log file.

### Firecracker daemon logs

The driver always configures Firecracker's built-in logger. Structured JSON logs are written to:

```
<chroot_base>/<exec_file_name>/<task_id>/root/firecracker.log
```

For example:

```bash
cat /srv/jailer/firecracker/<alloc_id>-<hash>/root/firecracker.log
```

Set `log_level` in the task config to control verbosity (defaults to `"Warning"`):

```hcl
config {
  log_level = "Debug"
}
```

> **Note:** `nomad alloc logs` only works for tasks that started successfully. If the task failed during startup (before reaching "started"), use `nomad alloc fs` instead to read the raw log files, and check the daemon log file in the chroot.

## Inspecting task events

Task events show the driver's lifecycle messages:

```bash
nomad alloc status <alloc_id>
```

Look for the **Recent Events** section. Common events:

| Event | Meaning |
|-------|---------|
| `Received` | Nomad received the allocation |
| `Task Setup` | Building task directory |
| `Driver Failure` | The driver returned an error (message has details) |
| `Not Restarting` | Nomad won't retry (unrecoverable error) |

## Agent log levels

For more verbose output from the Nomad agent and driver plugin, set the log level in your agent config:

```hcl
log_level = "DEBUG"   # or "TRACE" for maximum detail
```

`DEBUG` provides a good balance. `TRACE` is very noisy but useful for low-level protocol issues.

> **Note:** This controls the Nomad agent/driver log level, which is separate from the Firecracker daemon `log_level` set in the task config.
