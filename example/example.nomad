job "firecracker-example" {
  datacenters = ["dc1"]
  type        = "batch"

  group "vm-group" {

    network {
      mode = "bridge"
      port "http" {
        to = 80
      }
    }

    task "example-vm" {
      driver = "firecracker"

      config {

        boot_source {
          kernel_image_path = "/path/to/vmlinux.bin"
          boot_args         = "console=ttyS0 reboot=k panic=1 pci=off"
        }

        drive {
          path_on_host   = "/path/to/rootfs.ext4"
          is_root_device = true
          is_read_only   = false
        }

        drive {
          path_on_host = "/path/to/data.img"
          is_root_device = false
          is_read_only = true
        }

        network_interface {
          static_configuration {
            host_dev_name = "tap0"
            mac_address   = "02:fc:00:00:00:01"
          }
        }
      }

      resources {
        cpu    = 1024  # 1 vCPU
        memory = 512   # 512 MB
      }

      logs {
        max_files     = 3
        max_file_size = 10
      }
    }
  }
}

