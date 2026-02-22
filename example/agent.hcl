log_level = "TRACE"

plugin "nomad-driver-firecracker" {
  config {
    jailer {
      exec_file     = "/usr/local/bin/firecracker"
      jailer_binary = "/usr/local/bin/jailer"
    }
  }
}
