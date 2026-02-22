# Networking

## Modes

### Bridge Mode (recommended)

Use Nomad's built-in bridge networking:

```hcl
group "vm" {
  network {
    mode = "bridge"
  }
  task "firecracker" {
    driver = "firecracker"
    config { ... }
  }
}
```

When bridge mode is active and no manual `network_interface` blocks are configured, the driver automatically:

1. Enters the Nomad-allocated network namespace
2. Creates a TAP device (`tap0`) inside the namespace
3. Adds bidirectional TC ingress redirect filters between the veth (created by Nomad CNI) and the TAP
4. Passes `tap0` as `host_dev_name` to Firecracker
5. Launches the jailer with `--netns` pointing to the namespace

This replicates the [tc-redirect-tap](https://github.com/awslabs/tc-redirect-tap) CNI plugin mechanism. The setup is idempotent — task restarts within the same allocation reuse the existing TAP and filters.

**Traffic flow:**

```
VM → tap0 → (TC redirect) → veth → Nomad bridge → host network (and back)
```

**Requirements:**
- [CNI reference plugins](https://developer.hashicorp.com/nomad/docs/deploy#install-cni-reference-plugins) installed at `/opt/cni/bin/`
- Guest must configure its network interface (see [Guest Configuration](#guest-configuration) below)

### Host Mode

No automatic TAP setup. You must manually configure a `network_interface` block:

```hcl
config {
  network_interface {
    iface_id        = "eth0"
    host_dev_name   = "tap0"
    guest_mac       = "06:00:AC:10:00:02"
  }
}
```

The TAP device must be pre-created on the host.

## Guest Configuration

The guest VM must configure its network interface to use the IP allocated by Nomad. Common approaches:

- **systemd unit** (`fcnet.service`): A simple startup script that reads the expected IP/gateway and runs `ip addr add` / `ip route add`
- **cloud-init**: If the guest image supports it
- **DHCP**: If a DHCP server is available on the bridge network
- **Static config**: Baked into the rootfs image

The guest interface is typically `eth0` inside the VM.

For custom rootfs images, the guest VM must configure its network interface. Common approaches:

- **systemd unit** (`fcnet.service`): A startup script that configures `eth0` with the expected IP/gateway
- **DHCP**: If a DHCP server is available on the bridge network
- **Static config**: Baked into the rootfs image
