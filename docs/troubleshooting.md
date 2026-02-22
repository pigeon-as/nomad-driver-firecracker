# Troubleshooting

This guide covers common debugging techniques for the Firecracker Nomad driver.

## Checking allocation status

After submitting a job, check its status:

```bash
nomad job run e2e/jobs/echo.hcl
nomad job status echo
```

Look at the **Allocations** table. If the status is `failed`, note the allocation ID prefix (e.g. `41fa4a14`).

## Reading task logs

Nomad captures stdout and stderr from the jailer/firecracker process.

### List available log files

```bash
nomad alloc fs <alloc_id> alloc/logs/
```

Example output:

```
Mode        Size   Name
-rw-r--r--  357 B  echo-hello.stdout.0
-rw-r--r--  0 B    echo-hello.stderr.0
```

### Read stdout

```bash
nomad alloc fs <alloc_id> alloc/logs/echo-hello.stdout.0
```

### Read stderr

```bash
nomad alloc fs <alloc_id> alloc/logs/echo-hello.stderr.0
```

Stderr is where jailer and firecracker error messages appear (e.g. invalid instance ID, missing exec-file, etc.).

### Using `nomad alloc logs`

If the task reached the `started` state, you can also use:

```bash
nomad alloc logs <alloc_id>           # stdout
nomad alloc logs -stderr <alloc_id>   # stderr
```

> **Note:** `nomad alloc logs` only works for tasks that started successfully. If the task failed during startup (before reaching "started"), use `nomad alloc fs` instead to read the raw log files.

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
