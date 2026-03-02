job "full" {
  datacenters = ["dc1"]
  type        = "service"

  group "full" {
    # Bridge networking validates TAP + TC redirect setup. Port forwarding
    # to the guest requires pigeon-init (or equivalent) to configure eth0
    # inside the VM — without that, DNAT'd packets are dropped.
    network {
      mode = "bridge"
      port "http" {
        to = 5678
      }
    }

    # Health check validates the full traffic path:
    # curl → Nomad bridge DNAT → veth → TC redirect → tap0 → VM eth0 → http-echo
    service {
      name     = "http-echo"
      port     = "http"
      provider = "nomad"

      check {
        name     = "http-echo-alive"
        type     = "http"
        path     = "/"
        interval = "5s"
        timeout  = "2s"
      }
    }

    # Prestart: build ext4 rootfs from Docker image.
    # pigeon-init and kernel are placed in testdata/ by `make init` and `make kernel`.
    task "build-rootfs" {
      lifecycle {
        hook    = "prestart"
        sidecar = false
      }

      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["-c", <<EOF
set -euo pipefail
ROOTFS="${NOMAD_ALLOC_DIR}/rootfs.ext4"
WORK=$(mktemp -d)
trap 'rm -rf $WORK' EXIT

docker pull hashicorp/http-echo
CID=$(docker create hashicorp/http-echo)
docker export "$CID" > "$WORK/image.tar"
docker rm "$CID"

mkdir "$WORK/rootfs"
tar xf "$WORK/image.tar" -C "$WORK/rootfs"

truncate -s 64M "$ROOTFS"
mkfs.ext4 -F -d "$WORK/rootfs" "$ROOTFS"
EOF
        ]
      }

      resources {
        cpu    = 500
        memory = 256
      }
    }

    # Main task: boot Firecracker VM with all driver features enabled.
    #
    # - Bridge networking (TAP + TC redirect via Nomad CNI)
    # - pigeon-init (configures eth0 from MMDS, then execs workload)
    # - Vsock (host↔guest channel)
    # - Guest API (signal delivery over vsock — falls back to Ctrl+Alt+Del
    #   since http-echo has no vsock agent, but validates driver-side setup)
    # - Balloon (memory reclaim)
    # - MMDS (metadata service — auto-injects guest network config)
    # - Firecracker logger (log_level)
    #
    # pigeon-init runs as PID 1: mounts essential fs, queries MMDS for
    # IPConfigs, configures eth0, then starts the workload.
    task "http-echo" {
      driver = "firecracker"

      config {
        boot_source {
          kernel_image_path = "/tmp/testdata/vmlinux"
          initrd_path       = "/tmp/testdata/initrd.cpio"
          boot_args         = "console=ttyS0 reboot=k panic=1 pci=off"
        }

        drive {
          path_on_host   = "alloc/rootfs.ext4"
          is_root_device = true
        }

        vsock {
          guest_cid = 3
        }

        guest_api {
          port = 10000
        }

        balloon {
          amount_mib = 64
        }

        mmds {
          metadata = <<-META
{
  "ExecOverride": ["/http-echo", "-text=hello"],
  "Hostname": "http-echo"
}
META
        }

        log_level = "Info"
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
