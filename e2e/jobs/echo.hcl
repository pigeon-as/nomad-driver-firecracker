job "echo" {
  datacenters = ["dc1"]
  type        = "service"

  group "echo" {
    network {
      mode = "bridge"
      port "http" {
        to = 5678
      }
    }

    service {
      name     = "http-echo"
      port     = "http"
      provider = "nomad"
    }

    task "http-echo" {
      driver = "firecracker"

      config {
        boot_source {
          kernel_image_path = "/tmp/testdata/vmlinux"
          initrd_path       = "/tmp/testdata/initrd.cpio"
          boot_args         = "console=ttyS0 reboot=k panic=1 pci=off"
        }

        drive {
          path_on_host   = "/tmp/testdata/http-echo.ext4"
          is_root_device = true
        }

        mmds {
          metadata = <<-META
{
  "ExecOverride": ["/http-echo", "-text=hello"],
  "Hostname": "http-echo"
}
META
        }
      }

      resources {
        cpu    = 500
        memory = 256
      }

      logs {
        max_files     = 2
        max_file_size = 5
      }
    }
  }
}
