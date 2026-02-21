# VM Snapshots (Suspend/Resume)

The Firecracker driver supports VM suspend and resume via snapshots. Suspend pauses a running VM and saves its complete state to disk. Resume restores the VM in hundreds of milliseconds, much faster than a cold start.

## How It Works

### Suspend (SIGSTOP)

When you send `SIGSTOP` to a task, the driver:

1. Pauses the VM
2. Creates a snapshot capturing memory and CPU state
3. Stores snapshot files at `allocDir/snapshot`
4. Returns successfully

If snapshot creation fails, the VM resumes and returns an error.

```bash
nomad alloc signal -s SIGSTOP <alloc>
```

### Resume (SIGCONT)

When you send `SIGCONT` to a suspended task, the driver:

1. Checks if the VM is accessible
2. Resumes the VM from the paused state
3. Returns (best-effort, no timeout enforcement)

```bash
nomad alloc signal -s SIGCONT <alloc>
```

If resuming fails, the snapshot files remain on disk for troubleshooting.

## Storage and Lifecycle

Snapshots are stored at `allocDir/snapshot` and are cleaned up automatically when the task is destroyed. Snapshots are temporary and task-scoped only.

## Performance

| Operation | Time |
|-----------|------|
| **Resume from snapshot** | ~hundreds of milliseconds |
| **Cold start** | ~2+ seconds |

## Limitations

- **Network connections** may become stale after resume. Applications should handle reconnection on failures (see Network Connections section).
- **Root filesystem** is NOT reset on resume. Files written during suspend persist.
- **Clock skew** may occur for a few seconds after resume until NTP syncs.
- Snapshots require sufficient disk space in `allocDir/snapshot`.

## Network Connections After Resume

After resume, network connections may be stale. Remote systems (databases, APIs) might have closed their end of the connection.

**How to handle:**
- Implement connection retry/reconnect logic in your application
- Use connection pools that handle disconnects
- Shorten connection timeouts

Example:
```python
try:
    result = db.execute(query)
except ConnectionError:
    db.reconnect()
    result = db.execute(query)
```

## Usage Example

Suspend/resume enables fast task pause/recovery:

```
1. Task running normally
   ↓
2. Send SIGSTOP → VM suspends, snapshot created
   ↓
3. Send SIGCONT → VM resumes in ~100ms
```

## Monitoring

View logs with:
```bash
nomad alloc logs <alloc>
```

Look for:
- `"VM suspended with snapshot"` — successful suspend
- `"VM resumed from paused state"` — successful resume
- `"resume of paused VM failed"` — resume error occurred

## Troubleshooting

**Resume fails with "resume failed" but snapshot files exist:**
1. Check if Firecracker daemon is still running
2. Verify socket is accessible at `allocDir/jailer/<task_id>/root/run/firecracker.socket`
3. To recover: Send SIGTERM to destroy the task, then restart

**Snapshot creation fails (SIGSTOP error):**
1. Ensure sufficient disk space in `allocDir/snapshot`
2. Check Nomad agent logs for API errors
3. Verify Firecracker daemon is responsive
