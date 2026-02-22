log_level = "TRACE"

client {
  enabled = true
}

plugin "nomad-driver-firecracker" {
  config {
    jailer {
      exec_file     = "firecracker"
      jailer_binary = "jailer"
    }
  }
}
