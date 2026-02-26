# Signal Handling

Supported signals via `nomad alloc signal`:

## SIGTERM / SIGINT

Graceful shutdown. The driver tries three tiers in order:

1. **Vsock** (if `guest_api` configured) — `POST /v1/signals` with the signal number. Arch-independent, reaches the workload directly.
2. **Ctrl+Alt+Del** (x86_64 only) — Firecracker HTTP API. Guest kernel must have keyboard/serio support.
3. **Executor** — forwards the signal to the Firecracker VMM process.

```bash
nomad alloc signal -s SIGTERM <alloc>
nomad alloc signal -s SIGINT <alloc>
```

## StopTask (Nomad stop)

Nomad stop calls `StopTask()` with a timeout. The driver:

1. Attempts graceful shutdown — vsock SIGTERM (if `guest_api` configured), then Ctrl+Alt+Del
2. Polls for the VM to exit (up to the timeout)
3. If still running, passes remaining time to the executor's `Shutdown` (SIGTERM then SIGKILL)

## Other Signals

Any POSIX signal can be delivered via vsock when `guest_api` is configured. Without it, signals are forwarded to the Firecracker VMM process.

## SIGKILL

Not supported via HTTP API. Use `nomad alloc stop -no-shutdown-delay <alloc>` to skip the graceful period.

## See Also

- [guest-api.md](guest-api.md) — vsock guest agent configuration
