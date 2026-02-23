package machine

import (
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// BootSource describes the kernel and optional initrd for the VM.
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

func BootSourceHCLSpec() *hclspec.Spec {
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

// Drive describes a block device attached to the VM.
type Drive struct {
	PathOnHost   string `codec:"path_on_host"`
	IsRootDevice bool   `codec:"is_root_device"`
	IsReadOnly   bool   `codec:"is_read_only"`
}

func (d *Drive) Validate() error {
	if d == nil {
		return nil
	}
	if d.PathOnHost == "" {
		return errors.New("drive.path_on_host must be set")
	}
	return nil
}

func DriveHCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"path_on_host":   hclspec.NewAttr("path_on_host", "string", true),
		"is_root_device": hclspec.NewAttr("is_root_device", "bool", false),
		"is_read_only":   hclspec.NewAttr("is_read_only", "bool", false),
	})
}

func (d *Drive) ToSDK(id string) *models.Drive {
	if d == nil {
		return nil
	}
	return &models.Drive{
		DriveID:      strPtr(id),
		PathOnHost:   strPtr(d.PathOnHost),
		IsRootDevice: boolPtr(d.IsRootDevice),
		IsReadOnly:   boolPtr(d.IsReadOnly),
	}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
