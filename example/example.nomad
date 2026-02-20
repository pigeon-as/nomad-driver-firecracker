job "example" {
  datacenters = ["dc1"]
  type        = "batch"

  group "example" {
    network {
      mode = "bridge"
      port "http" { to = 80 }
    }

    task "example" {
      driver = "firecracker"

      config {

        boot_source {
          kernel_image_path = "/path/to/vmlinux"
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
    }
  }
}
