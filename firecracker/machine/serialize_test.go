package machine

import (
	"testing"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

func TestToSDK_NilConfig(t *testing.T) {
	_, err := ToSDK(nil, nil)
	must.Error(t, err)
}

func TestToSDK_MissingKernel(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}
	_, err := ToSDK(cfg, nil)
	must.Error(t, err)
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
		must.NoError(t, err, must.Sprintf("ToSDK with shares=%d", tt.shares))
		must.NotNil(t, vmCfg.MachineConfig, must.Sprintf("MachineConfig nil for shares=%d", tt.shares))
		must.NotNil(t, vmCfg.MachineConfig.VcpuCount, must.Sprintf("VcpuCount nil for shares=%d", tt.shares))
		must.EqOp(t, tt.wantVCPU, *vmCfg.MachineConfig.VcpuCount, must.Sprintf("shares=%d", tt.shares))
	}
}

func TestToSDK_MmdsConfig(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{{
			StaticConfiguration: &network.StaticNetworkConfiguration{
				HostDevName: "tap0",
			},
			Name: "eth0",
		}},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.SliceLen(t, 1, vmCfg.MmdsConfig.NetworkInterfaces)
	must.EqOp(t, "eth0", vmCfg.MmdsConfig.NetworkInterfaces[0])
}

func TestToSDK_MmdsConfigNil(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.Nil(t, vmCfg.MmdsConfig)
}

func TestToSDK_MmdsWithoutNetworkNoRouting(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		Mmds:       &Mmds{Metadata: `{"key":"value"}`},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.Nil(t, vmCfg.MmdsConfig)
}

func TestToSDK_MmdsBlockWithoutMetadata(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{{
			StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"},
		}},
		Mmds: &Mmds{Version: "V1"},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.NotNil(t, vmCfg.MmdsConfig.Version)
	must.EqOp(t, "V1", *vmCfg.MmdsConfig.Version)
}

func TestToSDK_MmdsInterfaceInvalid(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{Name: "primary", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
		},
		Mmds: &Mmds{Interface: "nonexistent"},
	}

	_, err := ToSDK(cfg, nil)
	must.Error(t, err)
}

func TestToSDK_MmdsInterfaceMatchesUnnamedNIC(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{{
			StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"},
		}},
		Mmds: &Mmds{Interface: "eth0"},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.EqOp(t, "eth0", vmCfg.MmdsConfig.NetworkInterfaces[0])
}

func TestToSDK_MmdsInterfaceMatchesSecondUnnamedNIC(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
			{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
		},
		Mmds: &Mmds{Interface: "eth1"},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.EqOp(t, "eth1", vmCfg.MmdsConfig.NetworkInterfaces[0])
}

func TestToSDK_MetadataAutoConfiguresMmds(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{{
			StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"},
		}},
		Mmds: &Mmds{Metadata: `{"key":"value"}`},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.NotNil(t, vmCfg.MmdsConfig.Version)
	must.EqOp(t, "V2", *vmCfg.MmdsConfig.Version)
	must.SliceLen(t, 1, vmCfg.MmdsConfig.NetworkInterfaces)
	must.EqOp(t, "eth0", vmCfg.MmdsConfig.NetworkInterfaces[0])
}

func TestToSDK_MmdsVersionV1(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{{
			StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"},
		}},
		Mmds: &Mmds{Metadata: `{"key":"value"}`, Version: "V1"},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.NotNil(t, vmCfg.MmdsConfig.Version)
	must.EqOp(t, "V1", *vmCfg.MmdsConfig.Version)
}

func TestToSDK_MmdsInterfaceOverride(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{Name: "primary", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
			{Name: "mgmt", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
		},
		Mmds: &Mmds{Metadata: `{"key":"value"}`, Interface: "mgmt"},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.SliceLen(t, 1, vmCfg.MmdsConfig.NetworkInterfaces)
	must.EqOp(t, "mgmt", vmCfg.MmdsConfig.NetworkInterfaces[0])
}

func TestToSDK_Balloon(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		Balloon:    &Balloon{AmountMiB: 128, DeflateOnOOM: true, StatsPollingInterval: 3},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.Balloon)
	must.EqOp(t, int64(128), *vmCfg.Balloon.AmountMib)
	must.True(t, *vmCfg.Balloon.DeflateOnOom)
	must.EqOp(t, int64(3), vmCfg.Balloon.StatsPollingIntervals)
}

func TestToSDK_NoBalloon(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.Nil(t, vmCfg.Balloon)
}

func TestToSDK_DriveRateLimiter(t *testing.T) {
	refillTime := int64(500)
	size := int64(524288)
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives: []Drive{{
			PathOnHost:   "/rootfs.ext4",
			IsRootDevice: true,
			RateLimiter: &models.RateLimiter{
				Bandwidth: &models.TokenBucket{
					RefillTime: &refillTime,
					Size:       &size,
				},
			},
		}},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.SliceLen(t, 1, vmCfg.Drives)
	must.NotNil(t, vmCfg.Drives[0].RateLimiter)
	must.EqOp(t, int64(524288), *vmCfg.Drives[0].RateLimiter.Bandwidth.Size)
}

func TestBootSource_ToSDK(t *testing.T) {
	b := &BootSource{KernelImagePath: "/vmlinux", BootArgs: "console=ttyS0", InitrdPath: "/initrd"}
	sdk := b.ToSDK()
	must.NotNil(t, sdk)
	must.EqOp(t, "/vmlinux", *sdk.KernelImagePath)
	must.EqOp(t, "console=ttyS0", sdk.BootArgs)
	must.EqOp(t, "/initrd", sdk.InitrdPath)
}

func TestBootSource_ToSDK_Nil(t *testing.T) {
	var b *BootSource
	must.Nil(t, b.ToSDK())
}

func TestDrive_ToSDK_RateLimiter(t *testing.T) {
	refillTime := int64(1000)
	size := int64(1048576)
	d := &Drive{
		PathOnHost:   "/rootfs.ext4",
		IsRootDevice: true,
		RateLimiter: &models.RateLimiter{
			Bandwidth: &models.TokenBucket{
				RefillTime: &refillTime,
				Size:       &size,
			},
		},
	}

	sdk := d.ToSDK("drive0")
	must.NotNil(t, sdk.RateLimiter)
	must.NotNil(t, sdk.RateLimiter.Bandwidth)
	must.EqOp(t, int64(1000), *sdk.RateLimiter.Bandwidth.RefillTime)
	must.EqOp(t, int64(1048576), *sdk.RateLimiter.Bandwidth.Size)
}

func TestDrive_ToSDK_NoRateLimiter(t *testing.T) {
	d := &Drive{PathOnHost: "/rootfs.ext4", IsRootDevice: true}
	sdk := d.ToSDK("drive0")
	must.Nil(t, sdk.RateLimiter)
}

func TestToSDK_NamedDrives(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives: []Drive{
			{Name: "root", PathOnHost: "/rootfs.ext4", IsRootDevice: true},
			{Name: "data", PathOnHost: "/data.ext4"},
		},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.SliceLen(t, 2, vmCfg.Drives)
	must.EqOp(t, "root", *vmCfg.Drives[0].DriveID)
	must.EqOp(t, "data", *vmCfg.Drives[1].DriveID)
}

func TestToSDK_UnnamedDrives(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives: []Drive{
			{PathOnHost: "/rootfs.ext4", IsRootDevice: true},
			{PathOnHost: "/data.ext4"},
		},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.SliceLen(t, 2, vmCfg.Drives)
	must.EqOp(t, "drive0", *vmCfg.Drives[0].DriveID)
	must.EqOp(t, "drive1", *vmCfg.Drives[1].DriveID)
}

func TestToSDK_NamedNICs(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{Name: "primary", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
			{Name: "mgmt", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
		},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.SliceLen(t, 2, vmCfg.NetworkInterfaces)
	must.EqOp(t, "primary", *vmCfg.NetworkInterfaces[0].IfaceID)
	must.EqOp(t, "mgmt", *vmCfg.NetworkInterfaces[1].IfaceID)
}

func TestToSDK_MetadataUsesNamedNIC(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{Name: "primary", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
		},
		Mmds: &Mmds{Metadata: `{"key":"value"}`},
	}

	vmCfg, err := ToSDK(cfg, nil)
	must.NoError(t, err)
	must.NotNil(t, vmCfg.MmdsConfig)
	must.SliceLen(t, 1, vmCfg.MmdsConfig.NetworkInterfaces)
	must.EqOp(t, "primary", vmCfg.MmdsConfig.NetworkInterfaces[0])
}

func TestToSDK_UnnamedNICs(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
			{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
		},
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if len(vmCfg.NetworkInterfaces) != 2 {
		t.Fatalf("expected 2 NICs, got %d", len(vmCfg.NetworkInterfaces))
	}
	if *vmCfg.NetworkInterfaces[0].IfaceID != "eth0" {
		t.Errorf("nic[0].IfaceID = %q, want \"eth0\"", *vmCfg.NetworkInterfaces[0].IfaceID)
	}
	if *vmCfg.NetworkInterfaces[1].IfaceID != "eth1" {
		t.Errorf("nic[1].IfaceID = %q, want \"eth1\"", *vmCfg.NetworkInterfaces[1].IfaceID)
	}
}

func TestToSDK_MetadataDefaultsToEth0ForUnnamedNIC(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
		},
		Mmds: &Mmds{Metadata: `{"key":"value"}`},
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

func TestBalloon_ToSDK_Values(t *testing.T) {
	b := &Balloon{AmountMiB: 256, DeflateOnOOM: true, StatsPollingInterval: 5}
	sdk := b.ToSDK()
	if sdk == nil {
		t.Fatal("expected non-nil SDK Balloon")
	}
	if *sdk.AmountMib != 256 {
		t.Errorf("AmountMib = %d, want 256", *sdk.AmountMib)
	}
	if *sdk.DeflateOnOom != true {
		t.Error("expected DeflateOnOom to be true")
	}
	if sdk.StatsPollingIntervals != 5 {
		t.Errorf("StatsPollingIntervals = %d, want 5", sdk.StatsPollingIntervals)
	}
}

func TestBalloon_ToSDK_Nil(t *testing.T) {
	var b *Balloon
	if b.ToSDK() != nil {
		t.Error("expected nil SDK Balloon for nil receiver")
	}
}

func TestToSDK_Vsock(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		Vsock:      &Vsock{GuestCID: 3},
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.Vsock == nil {
		t.Fatal("expected Vsock to be set")
	}
	if *vmCfg.Vsock.GuestCid != 3 {
		t.Errorf("Vsock.GuestCid = %d, want 3", *vmCfg.Vsock.GuestCid)
	}
	if *vmCfg.Vsock.UdsPath != VsockPath {
		t.Errorf("Vsock.UdsPath = %q, want %q", *vmCfg.Vsock.UdsPath, VsockPath)
	}
}

func TestToSDK_NoVsock(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.Vsock != nil {
		t.Error("expected Vsock to be nil when not configured")
	}
}

func TestVsock_ToSDK_Values(t *testing.T) {
	v := &Vsock{GuestCID: 42}
	sdk := v.ToSDK()
	if sdk == nil {
		t.Fatal("expected non-nil SDK Vsock")
	}
	if *sdk.GuestCid != 42 {
		t.Errorf("GuestCid = %d, want 42", *sdk.GuestCid)
	}
	if *sdk.UdsPath != VsockPath {
		t.Errorf("UdsPath = %q, want %q", *sdk.UdsPath, VsockPath)
	}
}

func TestVsock_ToSDK_Nil(t *testing.T) {
	var v *Vsock
	if v.ToSDK() != nil {
		t.Error("expected nil SDK Vsock for nil receiver")
	}
}
