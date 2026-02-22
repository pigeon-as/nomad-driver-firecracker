package drive

import (
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

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

func HCLSpec() *hclspec.Spec {
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
