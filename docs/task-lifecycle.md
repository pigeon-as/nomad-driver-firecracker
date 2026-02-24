# Task Lifecycle

## Starting a Task
1. Nomad calls `StartTask()` with config
2. Driver validates config and resources
3. If bridge networking is active and no manual network interfaces are configured, a TAP device with TC redirect is created inside the network namespace
4. If `snapshot_on_stop = true` and snapshot files exist from a previous run, the driver restores from snapshot (see [Snapshots](snapshots.md)); otherwise proceeds with cold boot
5. Jailer launches Firecracker in `<chroot_base>/<exec_file>/<allocID>-<hash>/root/`
6. Driver waits for the API socket to become ready (polls every 10ms, 3s timeout), matching the firecracker-go-sdk default
7. If restoring from snapshot, loads the snapshot and resumes the VM. If `metadata` is configured, MMDS data is re-pushed via `PUT /mmds`
8. For cold boot: configures the VM via sequential API calls (machine config, boot source, drives, network interfaces) then starts the instance. If `metadata` is configured, MMDS data is pushed via `PUT /mmds`
9. Socket path: `<chroot_base>/<exec_file>/<allocID>-<hash>/root/run/firecracker.socket`

## Stopping a Task

1. Nomad calls `StopTask()` with a single timeout deadline
2. If `snapshot_on_stop = true`: the driver pauses the VM, creates a snapshot, and saves snapshot files to `<task_dir>/snapshots/`
3. Otherwise: driver sends Ctrl+Alt+Del via Firecracker HTTP API and polls until the VM exits or the deadline expires
4. Remaining time budget is passed to the executor's `Shutdown` (SIGTERM then SIGKILL)
5. `DestroyTask` cleans up the jailer directory

## Task Recovery (after agent restart)

1. Nomad calls `RecoverTask()` with stored handle
2. Driver reattaches to existing executor process
3. Discovers the Firecracker socket path via glob and verifies readiness
4. Task resumes normal operation
