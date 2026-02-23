package machine

import (
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/boot_source"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/drive"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network_interface"
)

type Config struct {
	BootSource        *boot_source.BootSource
	Drives            []drive.Drive
	NetworkInterfaces network_interface.NetworkInterfaces
	MmdsConfig        *models.MmdsConfig
}
