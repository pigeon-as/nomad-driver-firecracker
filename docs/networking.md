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
- Guest must configure its network interface — the driver injects the IP/gateway into [MMDS](#mmds-microvm-metadata-service) automatically (see [Guest Configuration](#guest-configuration))

### Host Mode

No automatic TAP setup. You must manually configure a `network_interface` block:

```hcl
config {
  network_interface {
    static_configuration {
      host_dev_name = "tap0"
      mac_address   = "06:00:AC:10:00:02"
    }
  }
}
```

The TAP device must be pre-created on the host.

## Guest Configuration

The guest VM must configure its network interface to use the IP allocated by Nomad. The guest interface is typically `eth0` inside the VM.

When bridge networking is active, the driver reads the veth IP/mask and default gateway from the Nomad-allocated network namespace and injects them into MMDS as `IPConfigs`, matching pigeon-init's `RunConfig` format:

```json
{
  "IPConfigs": [
    {
      "Gateway": "172.26.64.1",
      "IP": "172.26.64.2",
      "Mask": 20
    }
  ]
}
```

The recommended guest-side approach is **pigeon-init** — a minimal init process that queries MMDS at `169.254.169.254`, unmarshals the data store as a `RunConfig`, and uses the `IPConfigs` to configure eth0.

## MMDS (Microvm Metadata Service)

The driver supports [MMDS](https://github.com/firecracker-microvm/firecracker/blob/main/docs/mmds/mmds-user-guide.md) for passing metadata to the guest VM.

### Automatic MMDS routing

MMDS routing is **automatically enabled** whenever the VM has at least one network interface (bridge mode or manual `network_interface`). You do not need to declare an `mmds {}` block for MMDS to work — the driver configures it on the first NIC using MMDS V2 by default.

### Driver-injected network config

In bridge mode, the driver automatically injects the guest network configuration (IP/mask and gateway read from the veth) into MMDS as a top-level `IPConfigs` field. This happens on every boot, including snapshot restore. See [Guest Configuration](#guest-configuration) for the MMDS payload layout.

The `IPConfigs` key is **reserved for driver use** — do not use it in user-provided metadata.

### User-provided metadata

To pass custom metadata to the guest, use the `mmds` block:

```hcl
config {
  mmds {
    metadata = <<EOF
{
  "instance-id": "i-1234567890abcdef0",
  "local-hostname": "my-vm"
}
EOF
  }
}
```

User metadata is merged with driver-injected keys into a single MMDS data store. The `IPConfigs` key is **reserved for driver use** — do not use it in user-provided metadata.

### MMDS version and interface override

The optional `version` and `interface` fields let you override defaults:

```hcl
config {
  mmds {
    version   = "V1"       # "V1" or "V2" (default: "V2")
    interface = "primary"  # must match a network_interface name
  }
}
```

### How it works

1. The driver validates user metadata JSON when the task starts
2. After the VM boots and the API socket is ready, the driver pushes combined metadata via `PUT /mmds`
3. MMDS routing is configured on the first NIC (or `mmds.interface` if specified) using MMDS V2 (or `mmds.version` if specified)
4. The guest retrieves metadata by querying `http://169.254.169.254/`

### Guest access

```bash
# MMDS v2 requires a token
TOKEN=$(curl -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-metadata-token-ttl-seconds: 21600")
curl -H "X-metadata-token: $TOKEN" http://169.254.169.254/
```

### Requirements

- At least one network interface must be configured (bridge mode or manual `network_interface`)
- User-provided metadata must be valid JSON
- The `IPConfigs` key is reserved — do not use it in user metadata
