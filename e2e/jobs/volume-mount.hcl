job "volume-mount" {
  datacenters = ["dc1"]
  type        = "batch"

  group "vm" {
    network {
      mode = "bridge"
    }

    volume "data" {
      type            = "host"
      source          = "test-data"
      access_mode     = "single-node-single-writer"
      attachment_mode = "block-device"
    }

    task "firecracker" {
      driver = "firecracker"

      volume_mount {
        volume      = "data"
        destination = "/data"
      }

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

        mmds {
          metadata = <<-META
{
  "ExecOverride": ["/bin/sh", "-c", "cat /data/marker.txt 2>/dev/null || echo NO_MARKER; echo volume-mount-ok > /data/marker.txt; cat /data/marker.txt; sync"],
  "Hostname": "volume-mount-test"
}
META
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
