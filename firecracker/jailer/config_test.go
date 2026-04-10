package jailer

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestBuildArgs(t *testing.T) {
	cfg := &JailerConfig{
		ExecFile:     "firecracker",
		JailerBinary: "jailer",
		ChrootBase:   "/srv/jailer",
	}

	uid := 1000
	gid := 1000
	params := &BuildParams{
		ID:            "task-1",
		UID:           &uid,
		GID:           &gid,
		NetNS:         "/var/run/netns/test",
		CgroupVersion: "2",
	}

	args, err := cfg.BuildArgs(params)
	must.NoError(t, err)
	must.True(t, len(args) > 0, must.Sprint("expected non-empty args"))

	// Verify critical args are present
	argSet := make(map[string]bool)
	for _, a := range args {
		argSet[a] = true
	}

	for _, required := range []string{"--id", "task-1", "--cgroup-version", "2", "--netns", "/var/run/netns/test"} {
		must.True(t, argSet[required], must.Sprintf("missing expected arg %q in %v", required, args))
	}
}

func TestBuildArgs_NilConfig(t *testing.T) {
	var cfg *JailerConfig
	_, err := cfg.BuildArgs(nil)
	must.Error(t, err)
}

func TestBuildArgs_MissingChrootBase(t *testing.T) {
	cfg := &JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer"}
	_, err := cfg.BuildArgs(nil)
	must.Error(t, err)
}

func TestJailerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *JailerConfig
		wantErr bool
	}{
		{"nil config", nil, false},
		{"valid", &JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer", ChrootBase: "/srv/jailer"}, false},
		{"missing exec_file", &JailerConfig{JailerBinary: "jailer", ChrootBase: "/srv/jailer"}, true},
		{"missing jailer_binary", &JailerConfig{ExecFile: "firecracker", ChrootBase: "/srv/jailer"}, true},
		{"missing chroot_base", &JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
			}
		})
	}
}
