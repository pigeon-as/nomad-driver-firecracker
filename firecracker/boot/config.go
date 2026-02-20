// Package boot provides configuration helpers for the Firecracker
// boot_source block that can appear within a Nomad task using the
// firecracker driver.  The types in this package mirror the upstream
// Firecracker API and include validation and conversion helpers so the
// driver itself can remain minimal.

package boot

import (
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
)

// BootSource corresponds to the Firecracker boot source configuration
// element.  Field names match the upstream API for familiarity.
//
// Example HCL:
//
//	boot_source {
//	  kernel_image_path = "/path/to/vmlinux"
//	  boot_args         = "console=ttyS0"
//	  initrd_path       = "/path/initrd.img"   # optional
//	}
type BootSource struct {
	KernelImagePath string `codec:"kernel_image_path"`
	BootArgs        string `codec:"boot_args"`
	InitrdPath      string `codec:"initrd_path"`
}

// Validate checks that the required fields are present and performs any
// semantic normalization.  It is safe to call on a nil receiver.
func (b *BootSource) Validate() error {
	if b == nil {
		return nil
	}
	if b.KernelImagePath == "" {
		return errors.New("boot_source.kernel_image_path must be provided")
	}
	return nil
}

// HCLSpec returns the HCL schema for a boot_source block.  It's just a
// placeholder that accepts an arbitrary object; decoding is done via
// mapstructure and validated by the type itself.
func HCLSpec() *hclspec.Spec {
	// not required at the outer level; existence is validated by TaskConfig
	return hclspec.NewBlock("boot_source", false, hclspec.NewObject(nil))
}

// ToSDK converts the BootSource value into the SDK equivalent.  A nil
// receiver returns nil.
func (b *BootSource) ToSDK() *models.BootSource {
	if b == nil {
		return nil
	}
	out := &models.BootSource{
		KernelImagePath: utils.String(b.KernelImagePath),
		BootArgs:        b.BootArgs,
	}
	if b.InitrdPath != "" {
		out.InitrdPath = b.InitrdPath
	}
	return out
}
