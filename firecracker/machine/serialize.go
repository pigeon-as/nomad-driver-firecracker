package machine

import (
	"errors"
	"fmt"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/go-openapi/strfmt"
	drivers "github.com/hashicorp/nomad/plugins/drivers"
)

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
			drvs[i] = d.ToSDK(fmt.Sprintf("drive%d", i))
		}
		vmCfg.Drives = drvs
	}
	if len(cfg.NetworkInterfaces) > 0 {
		vmCfg.NetworkInterfaces = cfg.NetworkInterfaces.ToSDK()
	} else {
		vmCfg.NetworkInterfaces = []*models.NetworkInterface{}
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
		version := "V2"
		vmCfg.MmdsConfig = &models.MmdsConfig{
			Version:           &version,
			NetworkInterfaces: []string{"eth0"},
		}
	}

	if err := vmCfg.Validate(strfmt.Default); err != nil {
		return vmCfg, err
	}

	return vmCfg, nil
}
