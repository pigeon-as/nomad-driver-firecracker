# Nomad Firecracker Driver

> **Warning:** This driver is experimental and not yet ready for production use.

A Nomad task driver plugin for running Firecracker microVMs.

## Requirements

- [Firecracker](https://github.com/firecracker-microvm/firecracker) and jailer binaries
- Linux kernel and root filesystem images for guest VMs
- [CNI plugins](https://github.com/containernetworking/plugins) at `/opt/cni/bin/` (required for bridge networking)

## Quick Start

```hcl
job "example" {
  group "vm-group" {

    network {
      mode = "bridge"
    }

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
plugin "nomad-driver-firecracker" {
  config {
    image_paths = ["/opt/vm-images"]
    
    jailer {
      exec_file     = "firecracker"
      jailer_binary = "jailer"
      chroot_base   = "/srv/jailer"
    }
  }
}
```

### Task Config

Required fields:
- `boot_source` - kernel image and boot args
- `drive` - at least one root drive with `is_root_device = true`

Optional fields:
- `network_interface` - manual tap device configuration (not needed for bridge mode; the driver automatically creates a TAP with TC redirect)
- `metadata` - JSON string pushed to the VM via [MMDS](https://github.com/firecracker-microvm/firecracker/blob/main/docs/mmds/mmds-user-guide.md) (requires networking)
- `snapshot_boot` - enable snapshot-based fast restart (see [Snapshots](docs/snapshots.md))

See [example job](example/example.nomad) for complete configuration.

## Documentation

- [Task Lifecycle](docs/task-lifecycle.md) - Start, stop, and recovery behavior
- [Signal Handling](docs/signals.md) - Signal forwarding and graceful shutdown
- [Networking](docs/networking.md) - Bridge mode, host mode, and guest configuration
- [Filesystem Layout](docs/filesystem.md) - Directory structure and file paths
- [Logging](docs/logs.md) - Daemon logs and guest console output
- [Snapshots](docs/snapshots.md) - Near-instant VM resume via snapshot boot
- [Troubleshooting](docs/troubleshooting.md) - Debugging common issues
