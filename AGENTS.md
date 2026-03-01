# Agent Instructions

## Project Overview

This is a HashiCorp Nomad task driver plugin for Firecracker micro-VMs. It follows the official Nomad driver plugin framework and must strictly adhere to patterns established in official HashiCorp drivers.

## Critical Rule: Adhere to Official Patterns

All code in this driver **must** follow the patterns, conventions, and idioms found in official Nomad drivers and documentation. You are **not allowed to deviate** from official patterns without an explicit, well-documented reason.

Do not invent unnecessary abstractions, emit events, or introduce mechanisms that official drivers do not use. When in doubt, check the reference drivers.

### Primary References (VM drivers — check these first)

- [Docker driver](https://github.com/hashicorp/nomad/tree/main/drivers/docker)
- [QEMU driver](https://github.com/hashicorp/nomad/tree/main/drivers/qemu)
- [Virt driver](https://github.com/hashicorp/nomad-driver-virt/)
- [Task driver authoring guide](https://developer.hashicorp.com/nomad/plugins/author/task-driver)
- [Skeleton driver plugin](https://github.com/hashicorp/nomad-skeleton-driver-plugin)

### Secondary References (if primary drivers don't clarify)

- [exec2 driver](https://github.com/hashicorp/nomad-driver-exec2)
- [Built-in drivers (rawexec, exec, java)](https://github.com/hashicorp/nomad/tree/main/drivers)

### Firecracker SDK

Before implementing custom Firecracker-specific logic, check the [firecracker-go-sdk](https://github.com/firecracker-microvm/firecracker-go-sdk) for existing functionality that can be reused.

### Firecracker Documentation

For jailer, host setup, and VM configuration, align with the [official Firecracker docs](https://github.com/firecracker-microvm/firecracker/tree/main/docs) (e.g. [production host setup](https://github.com/firecracker-microvm/firecracker/blob/main/docs/prod-host-setup.md)).

### What this means in practice

This driver has two layers:

1. **Framework code** (`driver.go`, `handle.go`, `state.go`) — must follow official driver patterns exactly. Plugin scaffolding, task lifecycle, state management, concurrency, `run()`, `TaskStatus()`, etc. When unsure, check the QEMU driver first.
2. **Domain code** (`guestapi/`, `machine/`, `snapshot/`, `jailer/`, `network/`) — Firecracker-specific logic in cleanly separated subpackages. Free to do what the domain needs.

Rules for framework code:
- `taskHandle` fields should match official drivers. Domain-specific client fields are fine (Docker stores `*client.Client`, Virt stores `VMGetter`) — just don't invent state that doesn't serve runtime communication with the workload.
- Do not emit events outside of `TaskEvents()`.
- Keep `run()` minimal — only update state fields, no side effects.
- Signal/stop paths may differ from executor-based drivers because VM guests can't receive host signals directly. Vsock and Ctrl+Alt+Del are the Firecracker equivalents of Docker's `ContainerKill`.

## Production Readiness Cross-Check

Before merging significant changes, cross-check every function in the driver against the official reference drivers (QEMU, Docker, Virt, Skeleton). Fetch the actual source code from each driver and compare function-by-function:

1. **Plugin scaffolding**: constructor, struct fields, PluginInfo, ConfigSchema, SetConfig
2. **Capabilities**: SendSignals, Exec, FSIsolation, NetIsolationModes, MountConfigs
3. **TaskState / taskHandle**: fields, lock patterns
4. **Fingerprint**: loop pattern, buildFingerprint logic
5. **StartTask**: full flow — decode, validate, executor setup, cleanup defer, launch, set state, go run()
6. **RecoverTask**: reattach, rebuild handle, go run()
7. **WaitTask / handleWait**: channel pattern, exec.Wait, select with dual-context
8. **StopTask**: graceful shutdown, timeout handling, exec.Shutdown
9. **DestroyTask**: force check, executor shutdown, plugin kill, cleanup
10. **InspectTask, TaskStats, TaskEvents, SignalTask, ExecTask**: delegation patterns
11. **run(), TaskStatus(), IsRunning()**: state update patterns
12. **taskStore**: concurrency patterns

Flag any deviation in framework code. Domain-specific behavior (vsock signals, Ctrl+Alt+Del, snapshot, jailer) is expected and does not need justification as long as it lives in the appropriate subpackage or is clearly Firecracker-specific.

## Build & Test

```bash
make build          # Build the plugin binary
make test           # Run unit tests
make e2e            # Run end-to-end tests (requires Firecracker + jailer)
```
