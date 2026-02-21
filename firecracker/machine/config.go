package machine

import (
	"errors"
	"fmt"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/boot_source"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/drive"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network_interface"
)

type Config struct {
	BootSource        *boot_source.BootSource
	Drives            []drive.Drive
	NetworkInterfaces network_interface.NetworkInterfaces
}

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
			return fmt.Errorf("drive[%d]: %v", i, err)
		}
	}

	if len(c.NetworkInterfaces) > 0 {
		if err := c.NetworkInterfaces.Validate(); err != nil {
			return err
		}
	}
	return nil
}
