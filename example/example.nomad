job "example" {
  datacenters = ["dc1"]
  type        = "batch"

  group "example" {
    network {
      mode = "bridge"
      port "http" { to = 80 }
    }

    task "example" {
      driver = "firecracker"

      config {

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
