log_level = "TRACE"

plugin "firecracker" {
  config {
    jailer {
      exec_file     = "/usr/local/bin/firecracker"
      jailer_binary = "/usr/local/bin/jailer"
    }
  }
}
