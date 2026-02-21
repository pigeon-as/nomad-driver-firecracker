# Signal Handling

Supported signals via `nomad alloc signal`:

## SIGTERM / SIGINT
Graceful shutdown. Sends Ctrl+Alt+Del to VM, allowing clean shutdown.
```bash
nomad alloc signal -s SIGTERM <alloc>
```

## SIGSTOP
Suspend VM with snapshot. Pauses VM and writes complete state (memory, CPU, I/O) to disk.
Snapshot lives only during task execution and is cleaned up on task destruction.

See [VM Snapshots](snapshots.md) for details on suspend/resume performance, limitations, and best practices.

```bash
nomad alloc signal -s SIGSTOP <alloc>
```

## SIGCONT
Resume paused VM from snapshot. Attempts to resume a VM that was previously suspended with SIGSTOP. 
Resumes from in-memory paused state in hundreds of milliseconds.

See [VM Snapshots](snapshots.md) for network connection recovery patterns and troubleshooting.

```bash
nomad alloc signal -s SIGCONT <alloc>
```

## SIGKILL
Not supported via HTTP API. Use `nomad alloc stop -no-shutdown <alloc>` for force-kill.
