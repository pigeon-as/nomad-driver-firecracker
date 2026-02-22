package boot_source

import (
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
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
	return hclspec.NewBlock("boot_source", true, hclspec.NewObject(map[string]*hclspec.Spec{
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
		KernelImagePath: strPtr(b.KernelImagePath),
		BootArgs:        b.BootArgs,
	}
	if b.InitrdPath != "" {
		out.InitrdPath = b.InitrdPath
	}
	return out
}

func strPtr(s string) *string { return &s }
