package firecracker

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/jailer"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/machine"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

var (
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"image_paths": hclspec.NewAttr("image_paths", "list(string)", false),
		"jailer":      hclspec.NewBlock("jailer", true, jailer.HCLSpec()),
	})

	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"boot_source":       machine.BootSourceHCLSpec(),
		"drive":             hclspec.NewBlockList("drive", machine.DriveHCLSpec()),
		"network_interface": hclspec.NewBlockList("network_interface", network.HCLSpec()),
		"balloon":           machine.BalloonHCLSpec(),
		"metadata":          hclspec.NewAttr("metadata", "string", false),
		"snapshot_on_stop":  hclspec.NewAttr("snapshot_on_stop", "bool", false),
	})

	capabilities = &drivers.Capabilities{
		SendSignals: true,
		Exec:        false,
		FSIsolation: drivers.FSIsolationImage,
		NetIsolationModes: []drivers.NetIsolationMode{
			drivers.NetIsolationModeHost,
			drivers.NetIsolationModeGroup,
		},
		MountConfigs: drivers.MountConfigSupportNone,
	}
)

type Config struct {
	// ImagePaths is a required allowlist of directories from which Firecracker
	// may load kernel, initrd, and drive images (in addition to the allocation directory).
	ImagePaths []string             `codec:"image_paths"`
	Jailer     *jailer.JailerConfig `codec:"jailer"`
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

	if len(c.ImagePaths) == 0 {
		return errors.New("image_paths is required: specify at least one directory containing kernel and drive images")
	}

	// Validate ImagePaths: must be non-empty absolute normalized paths
	for i, path := range c.ImagePaths {
		if path == "" {
			return fmt.Errorf("image_paths[%d]: path cannot be empty", i)
		}
		if !filepath.IsAbs(path) {
			return fmt.Errorf("image_paths[%d]: path must be absolute, got %q", i, path)
		}
		// Validate path is normalized
		normalized := filepath.Clean(path)
		if path != normalized {
			return fmt.Errorf("image_paths[%d]: path must be normalized, got %q (should be %q)", i, path, normalized)
		}
	}

	return nil
}

type TaskConfig struct {
	BootSource        *machine.BootSource       `codec:"boot_source"`
	Drives            []machine.Drive           `codec:"drive"`
	NetworkInterfaces network.NetworkInterfaces `codec:"network_interface"`
	Balloon           *machine.Balloon          `codec:"balloon"`
	Metadata          string                    `codec:"metadata"`
	SnapshotOnStop    bool                      `codec:"snapshot_on_stop"`
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

	if c.Balloon != nil {
		if err := c.Balloon.Validate(); err != nil {
			return err
		}
	}

	if c.Metadata != "" {
		if !json.Valid([]byte(c.Metadata)) {
			return errors.New("metadata must be valid JSON")
		}
	}

	return nil
}
