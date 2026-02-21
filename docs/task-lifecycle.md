# Task Lifecycle

## Starting a Task
1. Nomad calls `StartTask()` with config
2. Driver validates config and resources
3. Jailer launches Firecracker daemon in `taskDir/jailer/<task_id>/root/`
4. VM boots from specified kernel and root filesystem
5. Socket created at `taskDir/jailer/<task_id>/root/run/firecracker.socket`

## Stopping a Task
1. Nomad calls `StopTask()` with timeout
2. Driver attempts graceful shutdown via HTTP: sends Ctrl+Alt+Del to VM
3. Waits up to `timeout` for VM process to exit after Ctrl+Alt+Del
4. If VM does not exit within timeout, falls back to executor force-kill
5. Cleans up Jailer process

**Note:** To suspend a task with snapshot for faster resume, use `nomad alloc signal -s SIGSTOP <alloc>` instead. See [VM Snapshots](snapshots.md) for details.

## Suspending a Task (Snapshot/Resume)
1. Nomad sends signal `SIGSTOP` via driver's `StopTask()`
2. Driver pauses the VM via Firecracker API
3. Driver creates a snapshot of complete VM state (memory, CPU registers, I/O state)
4. Snapshot files stored at `allocDir/snapshot`
5. Task remains in suspended state, ready for fast resume

**Resume Performance:** ~hundreds of milliseconds vs ~2+ seconds for cold start.

See [VM Snapshots](snapshots.md) for suspend/resume semantics, network connection handling, and troubleshooting.

## Task Recovery (after agent restart)
1. Nomad calls `RecoverTask()` with stored handle
2. Driver reattaches to existing executor process
3. Verifies VM is responsive via HTTP health check
4. Restores socket path and snapshot state (if suspended)

**Snapshot Effect on Recovery:** If task was suspended, snapshot files in `allocDir/snapshot` are preserved and available for resume.
