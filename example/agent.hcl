log_level = "TRACE"

plugin "nomad-driver-firecracker" {
  config {
    image_paths = ["/var/lib/firecracker/images"]

    jailer {
      exec_file     = "/usr/local/bin/firecracker"
      jailer_binary = "/usr/local/bin/jailer"
    }
  }
}
