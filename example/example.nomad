# Copyright IBM Corp. 2019, 2025
# SPDX-License-Identifier: MPL-2.0

job "example" {
  datacenters = ["dc1"]
  type        = "batch"

  group "example" {
    network {
      mode = "bridge"  # triggers Nomad to create a network namespace
      port "http" { to = 80 }
    }

    task "hello-world" {
      driver = "firecracker"

      config {
        greeting = "hello"

        # each interface is declared via its own `network_interface` block.
        # this allows multiple devices without inventing a containerized list
        # type – the HCL parser handles repeated blocks automatically.
        network_interface {
          static_configuration {
            host_dev_name = "tap0"
            mac_address   = "02:fc:00:00:00:01"
          }
        }
      }
    }
  }
}
