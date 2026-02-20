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

	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"boot_source": boot.HCLSpec(),
		"drive":       hclspec.NewBlockList("drive", drive.HCLSpec()),
		"network_interface": hclspec.NewBlockList("network_interface", hclspec.NewObject(map[string]*hclspec.Spec{
			"allow_mmds": hclspec.NewAttr("allow_mmds", "bool", false),
			"static_configuration": hclspec.NewBlock("static_configuration", hclspec.NewObject(map[string]*hclspec.Spec{
				"host_dev_name": hclspec.NewAttr("host_dev_name", "string", true),
				"mac_address":   hclspec.NewAttr("mac_address", "string", false),
				"ip_configuration": hclspec.NewBlock("ip_configuration", hclspec.NewObject(map[string]*hclspec.Spec{
					"ip_addr":     hclspec.NewAttr("ip_addr", "string", true),
					"gateway":     hclspec.NewAttr("gateway", "string", true),
					"nameservers": hclspec.NewAttr("nameservers", "list(string)", false),
					"if_name":     hclspec.NewAttr("if_name", "string", false),
				})),
			})),
		})),
	})

	capabilities = &drivers.Capabilities{
		SendSignals: true,
		Exec:        false,
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
	c.Jailer.ChrootBaseDir = ""

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
