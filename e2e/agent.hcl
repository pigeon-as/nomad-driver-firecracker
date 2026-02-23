log_level = "DEBUG"

client {
  enabled = true
}

plugin "nomad-driver-firecracker" {
  config {
    image_paths = ["/tmp/firecracker-images"]

    jailer {
      exec_file     = "firecracker"
      jailer_binary = "jailer"
      chroot_base   = "/srv/jailer"
    }
  }
}
