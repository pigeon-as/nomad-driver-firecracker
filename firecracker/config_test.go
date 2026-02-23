package firecracker

import (
	"testing"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/boot_source"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/drive"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/jailer"
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
		{
			"valid snapshot_path",
			&Config{
				Jailer:       &jailer.JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"},
				ImagePaths:   []string{"/opt/images"},
				SnapshotPath: "/opt/vm-snapshots",
			},
			false,
		},
		{
			"relative snapshot_path",
			&Config{
				Jailer:       &jailer.JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"},
				ImagePaths:   []string{"/opt/images"},
				SnapshotPath: "relative/path",
			},
			true,
		},
		{
			"non-normalized snapshot_path",
			&Config{
				Jailer:       &jailer.JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"},
				ImagePaths:   []string{"/opt/images"},
				SnapshotPath: "/opt/vm-snapshots/../other",
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
	validBoot := &boot_source.BootSource{KernelImagePath: "vmlinux"}
	rootDrive := drive.Drive{PathOnHost: "/rootfs.ext4", IsRootDevice: true}
	dataDrive := drive.Drive{PathOnHost: "/data.ext4"}

	tests := []struct {
		name    string
		cfg     *TaskConfig
		wantErr bool
	}{
		{"nil config", nil, false},
		{"missing boot_source", &TaskConfig{Drives: []drive.Drive{rootDrive}}, true},
		{"missing drives", &TaskConfig{BootSource: validBoot}, true},
		{"no root device", &TaskConfig{BootSource: validBoot, Drives: []drive.Drive{dataDrive}}, true},
		{
			"two root devices",
			&TaskConfig{
				BootSource: validBoot,
				Drives: []drive.Drive{
					{PathOnHost: "/a.ext4", IsRootDevice: true},
					{PathOnHost: "/b.ext4", IsRootDevice: true},
				},
			},
			true,
		},
		{
			"valid minimal",
			&TaskConfig{BootSource: validBoot, Drives: []drive.Drive{rootDrive}},
			false,
		},
		{
			"valid metadata",
			&TaskConfig{BootSource: validBoot, Drives: []drive.Drive{rootDrive}, Metadata: `{"key":"value"}`},
			false,
		},
		{
			"invalid metadata JSON",
			&TaskConfig{BootSource: validBoot, Drives: []drive.Drive{rootDrive}, Metadata: `{not json}`},
			true,
		},
		{
			"empty metadata is valid",
			&TaskConfig{BootSource: validBoot, Drives: []drive.Drive{rootDrive}, Metadata: ""},
			false,
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
