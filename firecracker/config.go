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
	// on the client.  we expose a minimal jailer block under the plugin
	// since the fields are global to the driver; there is no per-task
	// configuration for the jailer and the chroot base is intentionally
	// omitted.
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

	// taskConfigSpec defines the HCL schema for task configuration.  It
	// resides in the root package so that callers can inspect it without
	// importing lower-level helpers.
	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"boot_source":       boot.HCLSpec(),
		"drive":             hclspec.NewBlockList("drive", drive.HCLSpec()),
		"network_interface": hclspec.NewBlockList("network_interface", hclspec.NewObject(nil)),
	})

	// capabilities indicates what optional features this driver supports
	capabilities = &drivers.Capabilities{
		SendSignals: true,
		Exec:        false,
	}
)

// Config contains configuration information for the plugin
// the jailer settings are provided once at agent startup and apply to
// every task; they are intentionally simple and the driver enforces that the
// chroot base directory lives inside the allocation work dir.
type Config struct {
	Jailer *jailer.JailerConfig `codec:"jailer"`
}

// Validate checks and normalises the plugin configuration.  in particular it
// makes sure the jailer section contains a valid exec binary and cleans up the
// chroot path so users can't override it.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.Jailer == nil {
		return errors.New("jailer configuration is required in plugin config")
	}
	// run the generic validation (defaults for binary/dir)
	if err := c.Jailer.Validate(); err != nil {
		return err
	}
	// ignore any user-supplied base dir; the driver calculates it per task
	c.Jailer.ChrootBaseDir = ""

	return nil
}

// TaskConfig contains all of the configuration a job can supply when using
// the firecracker driver.  Blocks are defined in separate packages to avoid
// leaking SDK types into the public API.
type TaskConfig struct {
	BootSource        *boot.BootSource          `codec:"boot_source"`
	Drives            []drive.Drive             `codec:"drive"`
	NetworkInterfaces network.NetworkInterfaces `codec:"network_interface"`
}

// Validate performs basic sanity checks on a TaskConfig.  Most of the heavy
// lifting is done by the subpackages.
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
