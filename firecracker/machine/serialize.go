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

	// MMDS routing is automatically enabled whenever networking exists.
	// This ensures the driver can always push metadata (e.g. guest network
	// config) without requiring the user to declare an mmds {} block.
	// The user's mmds.version and mmds.interface preferences are respected
	// when present.
	if len(cfg.NetworkInterfaces) > 0 {
		// Determine which NIC carries MMDS traffic.
		mmdsIface := ""
		if cfg.Mmds != nil {
			mmdsIface = cfg.Mmds.Interface
		}
		if mmdsIface == "" {
			// Default to the first NIC's resolved name.
			mmdsIface = cfg.NetworkInterfaces[0].Name
			if mmdsIface == "" {
				mmdsIface = "eth0"
			}
		} else {
			// Validate that the specified interface exists.
			found := false
			for idx, nic := range cfg.NetworkInterfaces {
				name := nic.Name
				if name == "" {
					name = fmt.Sprintf("eth%d", idx)
				}
				if name == mmdsIface {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("mmds.interface %q must match a configured network_interface name", mmdsIface)
			}
		}
		version := "V2"
		if cfg.Mmds != nil && cfg.Mmds.Version != "" {
			version = cfg.Mmds.Version
		}
		vmCfg.MmdsConfig = &models.MmdsConfig{
			Version:           &version,
			NetworkInterfaces: []string{mmdsIface},
		}
	}

	if err := vmCfg.Validate(strfmt.Default); err != nil {
		return vmCfg, err
	}

	return vmCfg, nil
}
