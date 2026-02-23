log_level = "TRACE"

plugin "nomad-driver-firecracker" {
  config {
    image_paths = ["/opt/vm-images"]

    jailer {
      exec_file     = "/usr/bin/firecracker"
      jailer_binary = "/usr/bin/jailer"
      chroot_base   = "/srv/jailer"
    }
  }
}
