//go:build !windows
// +build !windows

// Package vm contains helper functions for building Firecracker VM
// configurations from task-level settings.

package vm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/go-openapi/strfmt"
	drivers "github.com/hashicorp/nomad/plugins/drivers"
)

// ToSDK converts a VM Config to a firecracker-go-sdk FullVMConfiguration.
// It applies resource constraints if provided and validates the result.
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
	}
	if res != nil && res.NomadResources != nil {
		mc := &models.MachineConfiguration{}
		if res.NomadResources.Cpu.CpuShares > 0 {
			v := int64(res.NomadResources.Cpu.CpuShares)
			mc.VcpuCount = &v
		}
		if res.NomadResources.Memory.MemoryMB > 0 {
			m := int64(res.NomadResources.Memory.MemoryMB)
			mc.MemSizeMib = &m
		}
		if mc.VcpuCount != nil || mc.MemSizeMib != nil {
			vmCfg.MachineConfig = mc
		}
	}

	// SDK validation catches most problems, but it currently doesn't enforce
	// that a boot source is provided. We perform that check here so callers
	// can rely on ToSDK returning a usable object.
	if vmCfg.BootSource == nil || vmCfg.BootSource.KernelImagePath == nil || *vmCfg.BootSource.KernelImagePath == "" {
		return vmCfg, errors.New("boot_source.kernel_image_path must be provided")
	}

	if err := vmCfg.Validate(strfmt.Default); err != nil {
		return vmCfg, err
	}

	return vmCfg, nil
}

// Marshal serializes a VM Config to JSON bytes.
func Marshal(cfg *Config, res *drivers.Resources) ([]byte, error) {
	vmCfg, err := ToSDK(cfg, res)
	if err != nil {
		return nil, err
	}
	return json.Marshal(vmCfg)
}

// BuildVMConfig is the main entry point for building and writing a VM configuration.
// It converts the Config to SDK models, serializes to JSON, and writes to disk.
// Returns the serialized bytes for logging or debugging.
func BuildVMConfig(path string, cfg *Config, res *drivers.Resources) ([]byte, error) {
	data, err := Marshal(cfg, res)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, err
	}
	return data, nil
}
