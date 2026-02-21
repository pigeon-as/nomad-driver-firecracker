# Task Lifecycle

## Starting a Task
1. Nomad calls `StartTask()` with config
2. Driver validates config and resources
3. Jailer launches Firecracker daemon in `taskDir/jailer/<task_id>/root/`
4. VM boots from specified kernel and root filesystem
5. Socket created at `taskDir/jailer/<task_id>/root/run/firecracker.socket`

## Stopping a Task

1. Nomad calls `StopTask()` with timeout
2. Driver attempts graceful shutdown via Firecracker HTTP API: sends Ctrl+Alt+Del to VM
3. Executor enforces the timeout (SIGTERM then SIGKILL if needed)
4. Cleans up Jailer process and allocated resources

## Task Recovery (after agent restart)

1. Nomad calls `RecoverTask()` with stored handle
2. Driver reattaches to existing executor process
3. Verifies VM is responsive via socket health check
4. Task resumes normal operation
