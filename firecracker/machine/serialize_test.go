package machine

import (
	"testing"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/drivers"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
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

func TestToSDK_MetadataWithoutNetworkErrors(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		Metadata:   `{"key":"value"}`,
	}

	_, err := ToSDK(cfg, nil)
	if err == nil {
		t.Fatal("expected error when metadata is set without network interfaces")
	}
}

func TestToSDK_MetadataAutoConfiguresMmds(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{{
			StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"},
		}},
		Metadata: `{"key":"value"}`,
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.MmdsConfig == nil {
		t.Fatal("expected MmdsConfig to be set when metadata is provided")
	}
	if vmCfg.MmdsConfig.Version == nil || *vmCfg.MmdsConfig.Version != "V2" {
		t.Errorf("MmdsConfig.Version = %v, want V2", vmCfg.MmdsConfig.Version)
	}
	if len(vmCfg.MmdsConfig.NetworkInterfaces) != 1 || vmCfg.MmdsConfig.NetworkInterfaces[0] != "eth0" {
		t.Errorf("MmdsConfig.NetworkInterfaces = %v, want [eth0]", vmCfg.MmdsConfig.NetworkInterfaces)
	}
}

func TestToSDK_Balloon(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		Balloon:    &Balloon{AmountMiB: 128, DeflateOnOOM: true, StatsPollingInterval: 3},
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.Balloon == nil {
		t.Fatal("expected Balloon to be set")
	}
	if *vmCfg.Balloon.AmountMib != 128 {
		t.Errorf("Balloon.AmountMib = %d, want 128", *vmCfg.Balloon.AmountMib)
	}
	if *vmCfg.Balloon.DeflateOnOom != true {
		t.Error("expected DeflateOnOom to be true")
	}
	if vmCfg.Balloon.StatsPollingIntervals != 3 {
		t.Errorf("Balloon.StatsPollingIntervals = %d, want 3", vmCfg.Balloon.StatsPollingIntervals)
	}
}

func TestToSDK_NoBalloon(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.Balloon != nil {
		t.Error("expected Balloon to be nil when not configured")
	}
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
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if len(vmCfg.Drives) != 1 {
		t.Fatalf("expected 1 drive, got %d", len(vmCfg.Drives))
	}
	if vmCfg.Drives[0].RateLimiter == nil {
		t.Fatal("expected drive rate limiter to be set")
	}
	if *vmCfg.Drives[0].RateLimiter.Bandwidth.Size != 524288 {
		t.Errorf("drive rate limiter bandwidth size = %d, want 524288", *vmCfg.Drives[0].RateLimiter.Bandwidth.Size)
	}
}

func TestBootSource_ToSDK(t *testing.T) {
	b := &BootSource{KernelImagePath: "/vmlinux", BootArgs: "console=ttyS0", InitrdPath: "/initrd"}
	sdk := b.ToSDK()
	if sdk == nil {
		t.Fatal("expected non-nil SDK BootSource")
	}
	if *sdk.KernelImagePath != "/vmlinux" {
		t.Errorf("KernelImagePath = %s, want /vmlinux", *sdk.KernelImagePath)
	}
	if sdk.BootArgs != "console=ttyS0" {
		t.Errorf("BootArgs = %s, want console=ttyS0", sdk.BootArgs)
	}
	if sdk.InitrdPath != "/initrd" {
		t.Errorf("InitrdPath = %s, want /initrd", sdk.InitrdPath)
	}
}

func TestBootSource_ToSDK_Nil(t *testing.T) {
	var b *BootSource
	if b.ToSDK() != nil {
		t.Error("expected nil SDK BootSource for nil receiver")
	}
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
	if sdk.RateLimiter == nil {
		t.Fatal("expected rate limiter to be set")
	}
	if sdk.RateLimiter.Bandwidth == nil {
		t.Fatal("expected rate limiter bandwidth to be set")
	}
	if *sdk.RateLimiter.Bandwidth.RefillTime != 1000 {
		t.Errorf("RefillTime = %d, want 1000", *sdk.RateLimiter.Bandwidth.RefillTime)
	}
	if *sdk.RateLimiter.Bandwidth.Size != 1048576 {
		t.Errorf("Size = %d, want 1048576", *sdk.RateLimiter.Bandwidth.Size)
	}
}

func TestDrive_ToSDK_NoRateLimiter(t *testing.T) {
	d := &Drive{PathOnHost: "/rootfs.ext4", IsRootDevice: true}
	sdk := d.ToSDK("drive0")
	if sdk.RateLimiter != nil {
		t.Error("expected nil rate limiter")
	}
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
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if len(vmCfg.Drives) != 2 {
		t.Fatalf("expected 2 drives, got %d", len(vmCfg.Drives))
	}
	if *vmCfg.Drives[0].DriveID != "root" {
		t.Errorf("drive[0].DriveID = %q, want \"root\"", *vmCfg.Drives[0].DriveID)
	}
	if *vmCfg.Drives[1].DriveID != "data" {
		t.Errorf("drive[1].DriveID = %q, want \"data\"", *vmCfg.Drives[1].DriveID)
	}
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
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if *vmCfg.Drives[0].DriveID != "drive0" {
		t.Errorf("drive[0].DriveID = %q, want \"drive0\"", *vmCfg.Drives[0].DriveID)
	}
	if *vmCfg.Drives[1].DriveID != "drive1" {
		t.Errorf("drive[1].DriveID = %q, want \"drive1\"", *vmCfg.Drives[1].DriveID)
	}
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
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if len(vmCfg.NetworkInterfaces) != 2 {
		t.Fatalf("expected 2 NICs, got %d", len(vmCfg.NetworkInterfaces))
	}
	if *vmCfg.NetworkInterfaces[0].IfaceID != "primary" {
		t.Errorf("nic[0].IfaceID = %q, want \"primary\"", *vmCfg.NetworkInterfaces[0].IfaceID)
	}
	if *vmCfg.NetworkInterfaces[1].IfaceID != "mgmt" {
		t.Errorf("nic[1].IfaceID = %q, want \"mgmt\"", *vmCfg.NetworkInterfaces[1].IfaceID)
	}
}

func TestToSDK_MetadataUsesNamedNIC(t *testing.T) {
	cfg := &Config{
		BootSource: &BootSource{KernelImagePath: "vmlinux"},
		Drives:     []Drive{{PathOnHost: "/rootfs.ext4", IsRootDevice: true}},
		NetworkInterfaces: network.NetworkInterfaces{
			{Name: "primary", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
		},
		Metadata: `{"key":"value"}`,
	}

	vmCfg, err := ToSDK(cfg, nil)
	if err != nil {
		t.Fatalf("ToSDK: %v", err)
	}
	if vmCfg.MmdsConfig == nil {
		t.Fatal("expected MmdsConfig to be set")
	}
	if len(vmCfg.MmdsConfig.NetworkInterfaces) != 1 || vmCfg.MmdsConfig.NetworkInterfaces[0] != "primary" {
		t.Errorf("MmdsConfig.NetworkInterfaces = %v, want [primary]", vmCfg.MmdsConfig.NetworkInterfaces)
	}
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
		Metadata: `{"key":"value"}`,
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
