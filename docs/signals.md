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

- Attempts graceful shutdown via Ctrl+Alt+Del and waits (up to the StopTask timeout) for the guest VM to exit
- If the guest is still running when the timeout expires, invokes the executor's shutdown, which may send SIGTERM then SIGKILL as configured

## Other Signals

Other signals (SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2, etc.) are forwarded to the Firecracker VMM process. These signals are not specifically handled by the driver and may result in process termination or restart behavior depending on the signal.

## SIGKILL

Not supported via HTTP API. Use `nomad alloc stop -no-shutdown <alloc>` for force-kill.
