package vm

import (
	"errors"
	"fmt"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/boot"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/drive"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

type Config struct {
	BootSource        *boot.BootSource
	Drives            []drive.Drive
	NetworkInterfaces network.NetworkInterfaces
	Resources         interface{}
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
