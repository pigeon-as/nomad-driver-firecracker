package drive

import (
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
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
	return hclspec.NewBlock("drive", false, hclspec.NewObject(nil))
}

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
