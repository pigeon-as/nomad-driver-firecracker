package machine

import (
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

type Config struct {
	BootSource        *BootSource
	Drives            []Drive
	NetworkInterfaces network.NetworkInterfaces
	MmdsConfig        *models.MmdsConfig
	// Metadata is the raw JSON string for MMDS. When non-empty, ToSDK
	// validates that at least one network interface is configured and
	// sets MmdsConfig to V2 on the first interface ("eth0").
	Metadata string
}
