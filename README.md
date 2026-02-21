# Nomad Firecracker Driver

A Nomad task driver plugin for running Firecracker microVMs.

## Features

- Signal handling (SIGTERM, SIGSTOP, SIGCONT)
- Task recovery after agent restart
- Automatic log capture (daemon + guest console)
- Snapshot-based suspend/resume
- Resource stats

## Requirements

- [Firecracker](https://github.com/firecracker-microvm/firecracker) and jailer binaries
- Linux kernel and root filesystem images for guest VMs

## Quick Start

```hcl
job "example" {
  group "vm-group" {
    task "vm" {
      driver = "firecracker"
      
      config {
        boot_source {
          kernel_image_path = "/path/to/kernel"
          boot_args         = "console=ttyS0"
        }
        
        drive {
          path_on_host   = "/path/to/rootfs.ext4"
          is_root_device = true
          is_read_only   = false
        }
        
        network_interface {
          static_configuration {
            host_dev_name = "tap0"
            mac_address   = "02:fc:00:00:00:01"
          }
        }
      }
      
      resources {
        cpu    = 1024
        memory = 512
      }
    }
  }
}
```

## Configuration

### Plugin Config

In Nomad client configuration:
```hcl
plugin "firecracker" {
  config {
    jailer {
      exec_file     = "firecracker"
      jailer_binary = "jailer"
    }
  }
}
```

### Task Config

Required fields:
- `boot_source` - kernel image and boot args
- `drive` - at least one root drive with `is_root_device = true`

Optional fields:
- `network_interface` - tap-based networking (host device + optional MAC)

Note: Guest IP configuration is handled inside the VM (cloud-init, systemd-networkd, or custom init).

See [example job](example/example.nomad) for complete configuration.

## Documentation

- [Task Lifecycle](docs/task-lifecycle.md) - Start, stop, and recovery behavior
- [Signal Handling](docs/signals.md) - SIGTERM, SIGSTOP, SIGCONT usage
- [VM Snapshots](docs/snapshots.md) - Suspend/resume with snapshot-based state persistence
- [Filesystem Layout](docs/filesystem.md) - Directory structure and file paths
- [Logging](docs/logs.md) - Daemon logs and guest console output
