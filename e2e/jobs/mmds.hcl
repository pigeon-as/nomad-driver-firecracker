job "mmds" {
  datacenters = ["dc1"]
  type        = "batch"

  group "mmds" {
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

        mmds {
          metadata = <<EOF
{"instance-id":"test-123","local-hostname":"mmds-test"}
EOF
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
