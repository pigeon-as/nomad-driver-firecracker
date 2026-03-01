// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package machine

import (
	"encoding/json"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

// BuildMmdsContent merges user-provided MMDS metadata with driver-injected
// guest network configuration into a single value suitable for PUT /mmds.
//
// Returns nil when there is nothing to push (no user metadata and no network
// config), so the caller can skip the API call entirely.
//
// When bridge networking is active, the driver reads the IP/mask/gateway
// from the veth and injects an "IPConfigs" array matching pigeon-init's
// config.RunConfig format. pigeon-init's FetchMMDS unmarshals the entire
// MMDS data store as a RunConfig, so the fields must use PascalCase JSON
// tags (IP, Mask, Gateway).
//
// Layout example:
//
//	{
//	  "IPConfigs": [{"Gateway": "172.26.64.1", "IP": "172.26.64.2", "Mask": 20}],
//	  ...user keys...
//	}
func BuildMmdsContent(userMetadata string, guestNet *network.GuestNetworkConfig) interface{} {
	hasUser := userMetadata != ""
	hasNet := guestNet != nil && guestNet.IP != ""

	if !hasUser && !hasNet {
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

	return payload
}
