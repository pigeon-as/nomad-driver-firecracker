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
// When CNI networking is active, the driver reads all IP addresses assigned
// to the veth (IPv4 and/or IPv6 depending on the CNI IPAM configuration)
// and injects them as an "IPConfigs" array matching pigeon-init's
// config.RunConfig format. pigeon-init's FetchMMDS unmarshals the entire
// MMDS data store as a RunConfig, so the fields must use PascalCase JSON
// tags (IP, Mask, Gateway).
//
// When Nomad host volumes with block device paths are attached, the driver
// injects a "Mounts" array matching pigeon-init's config.Mount format.
//
// Both IPConfigs and Mounts are appended to any existing values from user
// metadata rather than overwriting, so the control plane (pigeon-api) can
// inject additional entries via the MMDS metadata string in the task config.
//
// Layout example (dual-stack CNI IPAM):
//
//	{
//	  "IPConfigs": [
//	    {"Gateway": "172.26.64.1", "IP": "172.26.64.2", "Mask": 20},
//	    {"Gateway": "fdaa:a1b2:c3d4:e5f6::1", "IP": "fdaa:a1b2:c3d4:e5f6::5", "Mask": 64}
//	  ],
//	  "Mounts": [{"DevicePath": "/dev/vdb", "MountPath": "/data"}],
//	  ...user keys...
//	}
func BuildMmdsContent(userMetadata string, guestNets []network.GuestNetworkConfig, mounts []GuestMount) interface{} {
	hasUser := userMetadata != ""
	hasNet := len(guestNets) > 0
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
	// Append to any existing IPConfigs from user metadata rather than
	// overwriting. Each CNI-assigned address (IPv4 and/or IPv6) becomes
	// a separate entry.
	for _, nc := range guestNets {
		if nc.IP == "" {
			continue
		}
		entry := map[string]interface{}{
			"IP":   nc.IP,
			"Mask": nc.Mask,
		}
		if nc.Gateway != "" {
			entry["Gateway"] = nc.Gateway
		}
		appendIPConfig(payload, entry)
	}

	// Inject Mounts matching pigeon-init's config.Mount struct.
	// Append to any existing Mounts from user metadata rather than
	// overwriting, same as IPConfigs.
	if hasMounts {
		mountList := make([]interface{}, len(mounts))
		for i, m := range mounts {
			mountList[i] = map[string]interface{}{
				"DevicePath": m.DevicePath,
				"MountPath":  m.MountPath,
			}
		}
		if existing, ok := payload["Mounts"].([]interface{}); ok {
			payload["Mounts"] = append(existing, mountList...)
		} else {
			payload["Mounts"] = mountList
		}
	}

	return payload
}

// appendIPConfig appends an IPConfig entry to the payload's "IPConfigs"
// array, creating it if it doesn't exist.
func appendIPConfig(payload map[string]interface{}, entry map[string]interface{}) {
	if existing, ok := payload["IPConfigs"].([]interface{}); ok {
		payload["IPConfigs"] = append(existing, entry)
	} else {
		payload["IPConfigs"] = []interface{}{entry}
	}
}
