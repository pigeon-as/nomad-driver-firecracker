package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/go-openapi/strfmt"
	drivers "github.com/hashicorp/nomad/plugins/drivers"
)

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
			if vcpuCount < 1 {
				vcpuCount = 1
			}
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
	}

	if err := vmCfg.Validate(strfmt.Default); err != nil {
		return vmCfg, err
	}

	return vmCfg, nil
}

func Marshal(cfg *Config, res *drivers.Resources) ([]byte, error) {
	vmCfg, err := ToSDK(cfg, res)
	if err != nil {
		return nil, err
	}
	return json.Marshal(vmCfg)
}

func BuildVMConfig(path string, cfg *Config, res *drivers.Resources) ([]byte, error) {
	data, err := Marshal(cfg, res)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0640); err != nil {
		return nil, err
	}
	return data, nil
}
