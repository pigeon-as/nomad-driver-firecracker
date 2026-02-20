package firecracker

import (
	"errors"

	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/jailer"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

var (
	// configSpec is the specification of the plugin's configuration
	// this is used to validate the configuration specified for the plugin
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

	// taskConfigSpec is the specification of the plugin's configuration for
	// a task
	// this is used to validated the configuration specified for the plugin
	// when a job is submitted.
	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		// TODO: define plugin's task configuration schema
		//
		// The schema should be defined using HCL specs and it will be used to
		// validate the task configuration provided by the user when they
		// submit a job.
		//
		// For example, for the schema below a valid task would be:
		//   job "example" {
		//     group "example" {
		//       task "say-hi" {
		//         driver = "hello-driver-plugin"
		//         config {
		//           greeting = "Hi"
		//         }
		//       }
		//     }
		//   }
		// arbitrary network configuration that is closely modeled after
		// Firecracker's own API.  We don't attempt to validate the inner
		// structure via HCL spec – users may pass a list of objects matching
		// the `firecracker-go-sdk` types – so we simply declare the attribute
		// as `any` and perform the heavy lifting later when decoding.
		"network_interface": hclspec.NewBlockList("network_interface", hclspec.NewObject(nil)),
	})

	// capabilities indicates what optional features this driver supports
	// this should be set according to the target run time.
	capabilities = &drivers.Capabilities{
		// TODO: set plugin's capabilities
		//
		// The plugin's capabilities signal Nomad which extra functionalities
		// are supported. For a list of available options check the docs page:
		// https://godoc.org/github.com/hashicorp/nomad/plugins/drivers#Capabilities
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

// TaskConfig contains configuration information for a task that runs with
// this plugin
//
// for the firecracker driver there is no task-specific jailer configuration
// – everything is driven from the plugin config. the greeting remains here
// only as an example.
type TaskConfig struct {

	// NetworkInterfaces corresponds to any number of `network_interface`
	// blocks inside the task `config`.  Those blocks are decoded into the
	// slice and then validated before a VM is created.  This mirrors the
	// Firecracker API while keeping Nomad-facing types clean.
	NetworkInterfaces network.NetworkInterfaces `codec:"network_interface"`
}
