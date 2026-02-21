package firecracker

import (
	"errors"
	"fmt"

	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/boot"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/drive"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/jailer"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

var (
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"jailer": hclspec.NewObject(map[string]*hclspec.Spec{
			"exec_file": hclspec.NewDefault(
				hclspec.NewAttr("exec_file", "string", false),
				hclspec.NewLiteral(`"firecracker"`),
			),
			"jailer_binary": hclspec.NewDefault(
				hclspec.NewAttr("jailer_binary", "string", false),
				hclspec.NewLiteral(`"jailer"`),
			),
		}),
	})

	rateLimiterSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"bandwidth": hclspec.NewBlock("bandwidth", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"refill_time":    hclspec.NewAttr("refill_time", "number", true),
			"size":           hclspec.NewAttr("size", "number", true),
			"one_time_burst": hclspec.NewAttr("one_time_burst", "number", false),
		})),
		"ops": hclspec.NewBlock("ops", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"refill_time":    hclspec.NewAttr("refill_time", "number", true),
			"size":           hclspec.NewAttr("size", "number", true),
			"one_time_burst": hclspec.NewAttr("one_time_burst", "number", false),
		})),
	})

	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"boot_source": boot.HCLSpec(),
		"drive":       hclspec.NewBlockList("drive", drive.HCLSpec()),
		"network_interface": hclspec.NewBlockList("network_interface", hclspec.NewObject(map[string]*hclspec.Spec{
			"static_configuration": hclspec.NewBlock("static_configuration", true, hclspec.NewObject(map[string]*hclspec.Spec{
				"host_dev_name": hclspec.NewAttr("host_dev_name", "string", true),
				"mac_address":   hclspec.NewAttr("mac_address", "string", false),
			})),
			"in_rate_limiter":  hclspec.NewBlock("in_rate_limiter", false, rateLimiterSpec),
			"out_rate_limiter": hclspec.NewBlock("out_rate_limiter", false, rateLimiterSpec),
		})),
	})

	capabilities = &drivers.Capabilities{
		SendSignals:       true,
		Exec:              false,
		FSIsolation:       drivers.FSIsolationChroot,
		NetIsolationModes: []drivers.NetIsolationMode{drivers.NetIsolationModeGroup},
	}
)

type Config struct {
	Jailer *jailer.JailerConfig `codec:"jailer"`
}

func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.Jailer == nil {
		return errors.New("jailer configuration is required in plugin config")
	}
	if err := c.Jailer.Validate(); err != nil {
		return err
	}

	return nil
}

type TaskConfig struct {
	BootSource        *boot.BootSource          `codec:"boot_source"`
	Drives            []drive.Drive             `codec:"drive"`
	NetworkInterfaces network.NetworkInterfaces `codec:"network_interface"`
}

func (c *TaskConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.BootSource == nil {
		return errors.New("boot_source block is required")
	}
	if err := c.BootSource.Validate(); err != nil {
		return err
	}

	if len(c.Drives) == 0 {
		return errors.New("at least one drive must be configured")
	}

	hasRootDevice := false
	for i, d := range c.Drives {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("drive[%d]: %v", i, err)
		}
		if d.IsRootDevice {
			if hasRootDevice {
				return errors.New("multiple drives marked as root device; only one drive can have is_root_device = true")
			}
			hasRootDevice = true
		}
	}
	if !hasRootDevice {
		return errors.New("exactly one drive must be marked as root device with is_root_device = true")
	}

	if len(c.NetworkInterfaces) > 0 {
		if err := c.NetworkInterfaces.Validate(); err != nil {
			return err
		}
	}
	return nil
}
