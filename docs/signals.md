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
- Driver waits up to StopTimeout for graceful exit
- If timeout expires, forcefully terminates Firecracker process

## Other Signals

Other signals (SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2, etc.) are forwarded to the Firecracker VMM process. These signals are not specifically handled by the driver and may result in process termination or restart behavior depending on the signal.

## SIGKILL

Not supported via HTTP API. Use `nomad alloc stop -no-shutdown <alloc>` for force-kill.
