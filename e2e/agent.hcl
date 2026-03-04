log_level = "DEBUG"

client {
  enabled = true

  # Volume mount tests require nomad-plugin-lvm built into this directory.
  # Plugin configuration (volume_group, thin_pool, etc.) is passed through
  # the volume definition's parameters {} block.
  host_volume_plugin_dir = "/tmp/nomad-plugins"
}

plugin "nomad-driver-firecracker" {
  config {
    image_paths = ["/tmp/testdata"]

    jailer {
      exec_file     = "firecracker"
      jailer_binary = "jailer"
      chroot_base   = "/srv/jailer"
    }
  }
}
