package machine

import (
	"errors"
	"fmt"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/go-openapi/strfmt"
	drivers "github.com/hashicorp/nomad/plugins/drivers"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }

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

func (d *Drive) ToSDK(id string) *models.Drive {
	if d == nil {
		return nil
	}
	out := &models.Drive{
		DriveID:      strPtr(id),
		PathOnHost:   strPtr(d.PathOnHost),
		IsRootDevice: boolPtr(d.IsRootDevice),
		IsReadOnly:   boolPtr(d.IsReadOnly),
	}
	if d.RateLimiter != nil {
		out.RateLimiter = d.RateLimiter
	}
	return out
}

func (b *Balloon) ToSDK() *models.Balloon {
	if b == nil {
		return nil
	}
	return &models.Balloon{
		AmountMib:    int64Ptr(b.AmountMiB),
		DeflateOnOom: boolPtr(b.DeflateOnOOM),
		// SDK field is StatsPollingIntervals (plural); our config uses the
		// singular StatsPollingInterval. Both represent seconds.
		StatsPollingIntervals: b.StatsPollingInterval,
	}
}

func (v *Vsock) ToSDK() *models.Vsock {
	if v == nil {
		return nil
	}
	udsPath := VsockPath
	return &models.Vsock{
		GuestCid: int64Ptr(v.GuestCID),
		UdsPath:  &udsPath,
	}
}

// ToSDK converts a driver Config into a firecracker-go-sdk
// FullVMConfiguration, suitable for sequential API calls.
func ToSDK(cfg *Config, res *drivers.Resources) (*models.FullVMConfiguration, error) {
	if cfg == nil {
		return nil, errors.New("vm config is required")
	}

	vmCfg := &models.FullVMConfiguration{}
	if cfg.BootSource != nil {
		vmCfg.BootSource = cfg.BootSource.ToSDK()
	}
	if len(cfg.Drives) > 0 {
		drvs := make([]*models.Drive, len(cfg.Drives))
		for i, d := range cfg.Drives {
			id := d.Name
			if id == "" {
				id = fmt.Sprintf("drive%d", i)
			}
			drvs[i] = d.ToSDK(id)
		}
		vmCfg.Drives = drvs
	}
	if len(cfg.NetworkInterfaces) > 0 {
		vmCfg.NetworkInterfaces = cfg.NetworkInterfaces.ToSDK()
	} else {
		vmCfg.NetworkInterfaces = []*models.NetworkInterface{}
	}
	if cfg.Balloon != nil {
		vmCfg.Balloon = cfg.Balloon.ToSDK()
	}
	if cfg.Vsock != nil {
		vmCfg.Vsock = cfg.Vsock.ToSDK()
	}
	if res != nil && res.NomadResources != nil {
		mc := &models.MachineConfiguration{}
		if res.NomadResources.Cpu.CpuShares > 0 {
			shares := res.NomadResources.Cpu.CpuShares
			vcpuCount := int64((shares + 1023) / 1024)
			mc.VcpuCount = &vcpuCount
		}
		if res.NomadResources.Memory.MemoryMB > 0 {
			m := int64(res.NomadResources.Memory.MemoryMB)
			mc.MemSizeMib = &m
		}
		if mc.VcpuCount != nil || mc.MemSizeMib != nil {
			vmCfg.MachineConfig = mc
		}
	}

	if vmCfg.BootSource == nil || vmCfg.BootSource.KernelImagePath == nil || *vmCfg.BootSource.KernelImagePath == "" {
		return nil, errors.New("boot_source.kernel_image_path must be provided")
	}

	if cfg.MmdsConfig != nil {
		vmCfg.MmdsConfig = cfg.MmdsConfig
	} else if cfg.Metadata != "" {
		// MMDS requires at least one network interface to route metadata.
		if len(cfg.NetworkInterfaces) == 0 {
			return nil, errors.New("metadata requires networking: configure bridge mode or a network_interface block")
		}
		// Use the first NIC's resolved name for MMDS routing.
		firstNIC := cfg.NetworkInterfaces[0].Name
		if firstNIC == "" {
			firstNIC = "eth0"
		}
		version := "V2"
		vmCfg.MmdsConfig = &models.MmdsConfig{
			Version:           &version,
			NetworkInterfaces: []string{firstNIC},
		}
	}

	if err := vmCfg.Validate(strfmt.Default); err != nil {
		return vmCfg, err
	}

	return vmCfg, nil
}
