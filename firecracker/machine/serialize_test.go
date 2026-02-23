package machine

import (
	"testing"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
)

func TestToSDK_NilConfig(t *testing.T) {
	_, err := ToSDK(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestToSDK_MissingKernel(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}
	_, err := ToSDK(cfg, nil)
	if err == nil {
		t.Fatal("expected error for missing kernel_image_path")
	}
}

func TestToSDK_CPUShareConversion(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}

	tests := []struct {
		shares   int64
		wantVCPU int64
	}{
		{500, 1},  // < 1024 → rounds to 1
		{1024, 1}, // exactly 1024 → 1
		{2048, 2}, // 2048 → 2
		{3000, 3}, // 3000+1023 = 4023 / 1024 = 3
	}

	for _, tt := range tests {
		res := &drivers.Resources{
			NomadResources: &structs.AllocatedTaskResources{
				Cpu:    structs.AllocatedCpuResources{CpuShares: tt.shares},
				Memory: structs.AllocatedMemoryResources{MemoryMB: 256},
			},
		}

		vmCfg, err := ToSDK(cfg, res)
		if err != nil {
			t.Fatalf("ToSDK with shares=%d: %v", tt.shares, err)
		}
		if vmCfg.MachineConfig == nil || vmCfg.MachineConfig.VcpuCount == nil {
			t.Fatalf("MachineConfig.VcpuCount is nil for shares=%d", tt.shares)
		}
		if got := *vmCfg.MachineConfig.VcpuCount; got != tt.wantVCPU {
			t.Errorf("shares=%d: vcpu_count = %d, want %d", tt.shares, got, tt.wantVCPU)
		}
	}
}

func TestToSDK_MmdsConfig(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		MmdsConfig: &models.MmdsConfig{
			NetworkInterfaces: []string{"eth0"},
		},
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.MmdsConfig == nil {
		t.Fatal("expected MmdsConfig to be set")
	}
	if len(vmCfg.MmdsConfig.NetworkInterfaces) != 1 || vmCfg.MmdsConfig.NetworkInterfaces[0] != "eth0" {
		t.Errorf("MmdsConfig.NetworkInterfaces = %v, want [eth0]", vmCfg.MmdsConfig.NetworkInterfaces)
	}
}

func TestToSDK_MmdsConfigNil(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.MmdsConfig != nil {
		t.Error("expected MmdsConfig to be nil when not configured")
	}
}
