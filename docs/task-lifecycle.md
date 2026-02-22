# Task Lifecycle

## Starting a Task
1. Nomad calls `StartTask()` with config
2. Driver validates config and resources
3. If bridge networking is active and no manual network interfaces are configured, a TAP device with TC redirect is created inside the network namespace
4. Jailer launches Firecracker in `taskDir/jailer/<exec_file_name>/<taskName>-<allocID>/root/`
5. VM boots from specified kernel and root filesystem
6. Socket created at `taskDir/jailer/<exec_file_name>/<taskName>-<allocID>/root/run/firecracker.socket`

## Stopping a Task

1. Nomad calls `StopTask()` with timeout
2. Driver sends Ctrl+Alt+Del via Firecracker HTTP API for graceful shutdown
3. Driver polls until the VM exits or the timeout expires
4. Remaining time budget is passed to the executor's `Shutdown` (SIGTERM then SIGKILL)
5. `DestroyTask` cleans up the jailer directory

## Task Recovery (after agent restart)

1. Nomad calls `RecoverTask()` with stored handle
2. Driver reattaches to existing executor process
3. Discovers the Firecracker socket path via glob and verifies readiness
4. Task resumes normal operation
