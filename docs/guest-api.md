# Guest API

Opt-in vsock guest agent integration for signal delivery, status queries, and command execution inside the VM.

## Configuration

Requires a `vsock` block and a `guest_api` block:

```hcl
config {
  vsock {
    guest_cid = 3
  }

  guest_api {
    port = 10000
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `port` | 10000 | Guest-side vsock port where the agent listens |

The guest must run an agent implementing the API below (e.g. [pigeon-init](https://github.com/pigeon-as/pigeon-init)).

## Signal Delivery

When `guest_api` is configured, the driver delivers signals through vsock first (arch-independent, supports any POSIX signal). Falls back to Ctrl+Alt+Del (x86_64 only, SIGTERM/SIGINT only), then to the executor. See [signals.md](signals.md).

## Endpoints

The agent exposes HTTP/1.1 over vsock using the Firecracker CONNECT protocol.

### GET /v1/status

Health check. Returns `{"ok": true}`.

### POST /v1/signals

Send a POSIX signal to the workload. Body: `{"signal": 15}`. Returns `{"ok": true}`.

### GET /v1/exit_code

Blocks until the workload exits. Returns `{"code": 0, "oom_killed": false}`.

### POST /v1/exec

Execute a one-shot command. Body: `{"cmd": ["ls", "-la"]}`.

Returns:

```json
{"exit_code": 0, "exit_signal": 0, "stdout": "...", "stderr": ""}
```
