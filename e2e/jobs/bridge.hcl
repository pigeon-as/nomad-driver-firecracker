job "bridge" {
  datacenters = ["dc1"]
  type        = "batch"

  group "bridge" {
    network {
      mode = "bridge"
    }

    task "firecracker" {
      driver = "firecracker"

      config {
        boot_source {
          kernel_image_path = "/tmp/firecracker-images/vmlinux"
          boot_args         = "console=ttyS0 reboot=k panic=1"
        }

        drive {
          path_on_host   = "/tmp/firecracker-images/rootfs.ext4"
          is_root_device = true
        }
      }

      logs {
        max_files     = 2
        max_file_size = 5
      }

      resources {
        cpu    = 500
        memory = 256
      }
    }
  }
}
