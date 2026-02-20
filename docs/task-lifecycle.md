# Task Lifecycle

## Starting a Task
1. Nomad calls `StartTask()` with config
2. Driver validates config and resources
3. Jailer launches Firecracker daemon in `allocDir/jailer/root/`
4. VM boots from specified kernel and root filesystem
5. Socket created at `allocDir/jailer/root/run/firecracker.socket`

## Stopping a Task
1. Nomad calls `StopTask()` 
2. Driver attempts graceful shutdown via HTTP: sends Ctrl+Alt+Del to VM
3. If timeout, falls back to executor force-kill
4. Cleans up Jailer process

## Task Recovery (after agent restart)
1. Nomad calls `RecoverTask()` with stored handle
2. Driver reattaches to existing executor process
3. Verifies VM is responsive via HTTP health check
4. Restores socket path and snapshot state (if suspended)
