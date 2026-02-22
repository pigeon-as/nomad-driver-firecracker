# Signal Handling

Supported signals via `nomad alloc signal`:

## SIGTERM / SIGINT

Graceful shutdown. Sends Ctrl+Alt+Del to the guest VM via the Firecracker API, allowing the guest OS to perform clean shutdown.

```bash
nomad alloc signal -s SIGTERM <alloc>
nomad alloc signal -s SIGINT <alloc>
```

**Behavior:**
- Firecracker HTTP API sends Ctrl+Alt+Del to guest
- Guest OS receives interrupt (typically triggers shutdown sequence)
- SignalTask returns after requesting shutdown; it does not wait for exit

## StopTask (Nomad stop)

Nomad stop calls `StopTask()` with a timeout. The driver:

- Attempts graceful shutdown via Ctrl+Alt+Del and polls (up to the timeout) for the VM to exit
- If the VM is still running when the timeout expires, the remaining time budget is passed to the executor's `Shutdown`, which sends SIGTERM then SIGKILL
- This follows a single-deadline approach matching the Docker driver pattern

## Other Signals

Other signals (SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2, etc.) are forwarded to the Firecracker VMM process. These signals are not specifically handled by the driver and may result in process termination or restart behavior depending on the signal.

## SIGKILL

Not supported via HTTP API. Use `nomad alloc stop -no-shutdown-delay <alloc>` to skip the graceful period, or `nomad job stop -purge <job>` for force-kill.
