package firecracker

import (
	"testing"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/jailer"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/machine"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{"nil config", nil, false},
		{"missing jailer", &Config{}, true},
		{
			"missing image_paths",
			&Config{Jailer: &jailer.JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"}},
			true,
		},
		{
			"valid minimal",
			&Config{
				Jailer:     &jailer.JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"},
				ImagePaths: []string{"/opt/images"},
			},
			false,
		},
		{
			"relative image_path",
			&Config{
				Jailer:     &jailer.JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"},
				ImagePaths: []string{"relative/path"},
			},
			true,
		},
		{
			"empty image_path",
			&Config{
				Jailer:     &jailer.JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"},
				ImagePaths: []string{""},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTaskConfig_Validate(t *testing.T) {
	validBoot := &machine.BootSource{KernelImagePath: "vmlinux"}
	rootDrive := machine.Drive{PathOnHost: "/rootfs.ext4", IsRootDevice: true}
	dataDrive := machine.Drive{PathOnHost: "/data.ext4"}

	tests := []struct {
		name    string
		cfg     *TaskConfig
		wantErr bool
	}{
		{"nil config", nil, false},
		{"missing boot_source", &TaskConfig{Drives: []machine.Drive{rootDrive}}, true},
		{"missing drives", &TaskConfig{BootSource: validBoot}, true},
		{"no root device", &TaskConfig{BootSource: validBoot, Drives: []machine.Drive{dataDrive}}, true},
		{
			"two root devices",
			&TaskConfig{
				BootSource: validBoot,
				Drives: []machine.Drive{
					{PathOnHost: "/a.ext4", IsRootDevice: true},
					{PathOnHost: "/b.ext4", IsRootDevice: true},
				},
			},
			true,
		},
		{
			"valid minimal",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}},
			false,
		},
		{
			"valid metadata",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Metadata: `{"key":"value"}`},
			false,
		},
		{
			"invalid metadata JSON",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Metadata: `{not json}`},
			true,
		},
		{
			"empty metadata is valid",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Metadata: ""},
			false,
		},
		{
			"valid balloon",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Balloon: &machine.Balloon{AmountMiB: 128, DeflateOnOOM: true}},
			false,
		},
		{
			"balloon negative amount",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Balloon: &machine.Balloon{AmountMiB: -1}},
			true,
		},
		{
			"nil balloon is valid",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Balloon: nil},
			false,
		},
		{
			"valid log_level Debug",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, LogLevel: "Debug"},
			false,
		},
		{
			"valid log_level Warning",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, LogLevel: "Warning"},
			false,
		},
		{
			"invalid log_level",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, LogLevel: "verbose"},
			true,
		},
		{
			"empty log_level uses default",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, LogLevel: ""},
			false,
		},
		{
			"nil vsock is valid",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Vsock: nil},
			false,
		},
		{
			"valid vsock guest_cid",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Vsock: &machine.Vsock{GuestCID: 3}},
			false,
		},
		{
			"vsock guest_cid too low",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Vsock: &machine.Vsock{GuestCID: 2}},
			true,
		},
		{
			"vsock guest_cid too high",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, Vsock: &machine.Vsock{GuestCID: 4294967296}},
			true,
		},
		{
			"all drives named",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{
				{Name: "root", PathOnHost: "/rootfs.ext4", IsRootDevice: true},
				{Name: "data", PathOnHost: "/data.ext4"},
			}},
			false,
		},
		{
			"no drives named",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{
				{PathOnHost: "/rootfs.ext4", IsRootDevice: true},
				{PathOnHost: "/data.ext4"},
			}},
			false,
		},
		{
			"mixed drive names",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{
				{Name: "root", PathOnHost: "/rootfs.ext4", IsRootDevice: true},
				{PathOnHost: "/data.ext4"},
			}},
			true,
		},
		{
			"duplicate drive names",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{
				{Name: "disk", PathOnHost: "/rootfs.ext4", IsRootDevice: true},
				{Name: "disk", PathOnHost: "/data.ext4"},
			}},
			true,
		},
		{
			"all network interfaces named",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, NetworkInterfaces: network.NetworkInterfaces{
				{Name: "eth0", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
				{Name: "eth1", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
			}},
			false,
		},
		{
			"no network interface named",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, NetworkInterfaces: network.NetworkInterfaces{
				{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
				{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
			}},
			false,
		},
		{
			"mixed network interface names",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, NetworkInterfaces: network.NetworkInterfaces{
				{Name: "eth0", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
				{StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
			}},
			true,
		},
		{
			"duplicate network interface names",
			&TaskConfig{BootSource: validBoot, Drives: []machine.Drive{rootDrive}, NetworkInterfaces: network.NetworkInterfaces{
				{Name: "eth0", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap0"}},
				{Name: "eth0", StaticConfiguration: &network.StaticNetworkConfiguration{HostDevName: "tap1"}},
			}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
