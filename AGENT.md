# Agent Instructions

## Project Overview

This is a HashiCorp Nomad task driver plugin for Firecracker micro-VMs. It follows the official Nomad driver plugin framework and must strictly adhere to patterns established in official HashiCorp drivers.

## Critical Rule: Adhere to Official Patterns

All code in this driver **must** follow the patterns, conventions, and idioms found in official Nomad drivers and documentation. You are **not allowed to deviate** from official patterns without an explicit, well-documented reason.

Do not invent custom abstractions, add fields to `taskHandle` or `taskStore`, emit events, or introduce mechanisms that official drivers do not use. When in doubt, check the reference drivers.

### Primary References (VM drivers — check these first)

- [Docker driver](https://github.com/hashicorp/nomad/tree/main/drivers/docker)
- [QEMU driver](https://github.com/hashicorp/nomad/tree/main/drivers/qemu)
- [Virt driver](https://github.com/hashicorp/nomad-driver-virt/)
- [Task driver authoring guide](https://developer.hashicorp.com/nomad/plugins/author/task-driver)
- [Skeleton driver plugin](https://github.com/hashicorp/nomad-skeleton-driver-plugin)

### Secondary References (if primary drivers don't clarify)

- [exec2 driver](https://github.com/hashicorp/nomad-driver-exec2)
- [Built-in drivers (rawexec, exec, java)](https://github.com/hashicorp/nomad/tree/main/drivers)

### What this means in practice

- Do not add fields to `taskHandle`, `taskStore`, or `TaskState` unless those fields exist in official drivers.
- Do not emit events outside of `TaskEvents()` — official executor-based drivers don't emit lifecycle events.
- Keep `run()` minimal — only update state fields, no side effects.
- Domain-specific logic (jailer, Firecracker API) should be cleanly separated from driver framework code.
- When unsure about a pattern, check the QEMU driver first — it's the closest match to this project.

## Build & Test

```bash
make build          # Build the plugin binary
make test           # Run unit tests
make e2e            # Run end-to-end tests (requires Firecracker + jailer)
```
