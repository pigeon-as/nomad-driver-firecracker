name         = "test-data"
type         = "host"
plugin_id    = "nomad-plugin-lvm"
capacity_min = "64MB"
capacity_max = "64MB"

capability {
  access_mode     = "single-node-single-writer"
  attachment_mode = "block-device"
}

parameters {
  type         = "persistent"
  mode         = "block"
  volume_group = "e2e-vg"
  thin_pool    = "thinpool"
}
