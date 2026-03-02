job "basic" {
  datacenters = ["dc1"]
  type        = "batch"

  group "basic" {
    task "firecracker" {
      driver = "firecracker"

      config {
        boot_source {
          kernel_image_path = "/tmp/testdata/vmlinux"
          initrd_path       = "/tmp/testdata/initrd.cpio"
          boot_args         = "console=ttyS0 reboot=k panic=1 pci=off"
        }

        drive {
          path_on_host   = "/tmp/testdata/rootfs.ext4"
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
