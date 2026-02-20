// Package drive provides configuration helpers for the Firecracker
// drive block that can appear multiple times in a Nomad task using the
// firecracker driver.

package drive

import (
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
)

// Drive represents a single drive device attached to the microVM.  Only a
// minimal subset of the upstream API is exposed; more fields can be added
// later as needed.
//
// Example HCL:
//
//	drive {
//	  path_on_host   = "/path/to/rootfs.img"
//	  is_root_device = true
//	  is_read_only   = false
//	}
type Drive struct {
	PathOnHost   string `codec:"path_on_host"`
	IsRootDevice bool   `codec:"is_root_device"`
	IsReadOnly   bool   `codec:"is_read_only"`
}

// Validate performs a quick semantic check of the configuration.  A nil
// receiver is valid.
func (d *Drive) Validate() error {
	if d == nil {
		return nil
	}
	if d.PathOnHost == "" {
		return errors.New("drive.path_on_host must be set")
	}
	return nil
}

// HCLSpec returns the HCL schema for a single drive block.  The driver
// registers a repeated block using this spec.
func HCLSpec() *hclspec.Spec {
	return hclspec.NewBlock("drive", false, hclspec.NewObject(nil))
}

// ToSDK turns the Drive into the corresponding SDK model.  The caller must
// provide a stable ID (typically an index-based string) which the SDK
// requires.
func (d *Drive) ToSDK(id string) *models.Drive {
	if d == nil {
		return nil
	}
	return &models.Drive{
		DriveID:      utils.String(id),
		PathOnHost:   utils.String(d.PathOnHost),
		IsRootDevice: utils.Bool(d.IsRootDevice),
		IsReadOnly:   utils.Bool(d.IsReadOnly),
	}
}
