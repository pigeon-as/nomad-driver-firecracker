//go:build !windows
// +build !windows

package vm

import (
	"errors"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/boot"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/drive"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

// Config contains all the Firecracker VM configuration derived from task-level settings.
// It combines boot source, drives, network interfaces, and resource constraints.
type Config struct {
	BootSource        *boot.BootSource
	Drives            []drive.Drive
	NetworkInterfaces network.NetworkInterfaces
	// Resources are optional; if provided they are used to set vcpu/memory limits
	Resources interface{} // *drivers.Resources
}

// Validate performs basic sanity checks on a VM Config.
// Most validation is delegated to individual components.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.BootSource == nil {
		return errors.New("boot_source is required")
	}
	if err := c.BootSource.Validate(); err != nil {
		return err
	}

	for i, d := range c.Drives {
		if err := d.Validate(); err != nil {
			return err
		}
	}

	if len(c.NetworkInterfaces) > 0 {
		if err := c.NetworkInterfaces.Validate(); err != nil {
			return err
		}
	}
	return nil
}
