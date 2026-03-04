// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package machine

import (
	"encoding/json"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

// GuestMount describes a block device mount that pigeon-init should
// perform inside the guest. The driver builds this from Nomad volume
// mounts whose host path is a block device.
type GuestMount struct {
	DevicePath string `json:"DevicePath"` // e.g. "/dev/vdb"
	MountPath  string `json:"MountPath"`  // e.g. "/data"
}

// BuildMmdsContent merges user-provided MMDS metadata with driver-injected
// guest network configuration and volume mount configuration into a single
// value suitable for PUT /mmds.
//
// Returns nil when there is nothing to push (no user metadata, no network
// config, and no mounts), so the caller can skip the API call entirely.
//
// When bridge networking is active, the driver reads the IP/mask/gateway
// from the veth and injects an "IPConfigs" array matching pigeon-init's
// config.RunConfig format. pigeon-init's FetchMMDS unmarshals the entire
// MMDS data store as a RunConfig, so the fields must use PascalCase JSON
// tags (IP, Mask, Gateway).
//
// When Nomad host volumes with block device paths are attached, the driver
// injects a "Mounts" array matching pigeon-init's config.Mount format.
//
// Layout example:
//
//	{
//	  "IPConfigs": [{"Gateway": "172.26.64.1", "IP": "172.26.64.2", "Mask": 20}],
//	  "Mounts": [{"DevicePath": "/dev/vdb", "MountPath": "/data"}],
//	  ...user keys...
//	}
func BuildMmdsContent(userMetadata string, guestNet *network.GuestNetworkConfig, mounts []GuestMount) interface{} {
	hasUser := userMetadata != ""
	hasNet := guestNet != nil && guestNet.IP != ""
	hasMounts := len(mounts) > 0

	if !hasUser && !hasNet && !hasMounts {
		return nil
	}

	payload := make(map[string]interface{})

	// Merge user-provided metadata (Validate ensures this is a JSON object).
	if hasUser {
		_ = json.Unmarshal([]byte(userMetadata), &payload)
	}

	// Inject IPConfigs matching pigeon-init's config.IPConfig struct.
	if hasNet {
		payload["IPConfigs"] = []interface{}{
			map[string]interface{}{
				"Gateway": guestNet.Gateway,
				"IP":      guestNet.IP,
				"Mask":    guestNet.Mask,
			},
		}
	}

	// Inject Mounts matching pigeon-init's config.Mount struct.
	if hasMounts {
		mountList := make([]interface{}, len(mounts))
		for i, m := range mounts {
			mountList[i] = map[string]interface{}{
				"DevicePath": m.DevicePath,
				"MountPath":  m.MountPath,
			}
		}
		payload["Mounts"] = mountList
	}

	return payload
}
