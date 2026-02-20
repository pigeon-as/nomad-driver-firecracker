package boot

import (
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
)

type BootSource struct {
	KernelImagePath string `codec:"kernel_image_path"`
	BootArgs        string `codec:"boot_args"`
	InitrdPath      string `codec:"initrd_path"`
}

func (b *BootSource) Validate() error {
	if b == nil {
		return nil
	}
	if b.KernelImagePath == "" {
		return errors.New("boot_source.kernel_image_path must be provided")
	}
	return nil
}

func HCLSpec() *hclspec.Spec {
	return hclspec.NewBlock("boot_source", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"kernel_image_path": hclspec.NewAttr("kernel_image_path", "string", true),
		"boot_args":         hclspec.NewAttr("boot_args", "string", false),
		"initrd_path":       hclspec.NewAttr("initrd_path", "string", false),
	}))
}

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
