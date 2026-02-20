# Copyright IBM Corp. 2019, 2025
# SPDX-License-Identifier: MPL-2.0

log_level = "TRACE"

plugin "firecracker" {
  config {
    jailer {
      exec_file     = "/usr/local/bin/firecracker"
      jailer_binary = "/usr/local/bin/jailer"
    }
  }
}
