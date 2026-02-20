# Signal Handling

Supported signals via `nomad alloc signal`:

## SIGTERM / SIGINT
Graceful shutdown. Sends Ctrl+Alt+Del to VM, allowing clean shutdown.
```bash
nomad alloc signal -s SIGTERM <alloc>
```

## SIGSTOP
Suspend VM with snapshot. Pauses VM and writes memory/state to `allocDir/snapshot/`.
Snapshot lives only during task execution and is cleaned up on task destruction.
```bash
nomad alloc signal -s SIGSTOP <alloc>
```

## SIGCONT
Resume from snapshot. Resumes previously paused VM. Requires prior SIGSTOP.
```bash
nomad alloc signal -s SIGCONT <alloc>
```

## SIGKILL
Not supported via HTTP API. Use `nomad alloc stop -no-shutdown <alloc>` for force-kill.
