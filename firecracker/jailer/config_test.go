package jailer

import (
	"testing"
)

func TestBuildArgs(t *testing.T) {
	cfg := &JailerConfig{
		ExecFile:     "firecracker",
		JailerBinary: "jailer",
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

	args, err := cfg.BuildArgs("/alloc/task", params, "--config-file", "/vmconfig.json")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}

	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}

	// Verify critical args are present
	argSet := make(map[string]bool)
	for _, a := range args {
		argSet[a] = true
	}

	for _, required := range []string{"--id", "task-1", "--cgroup-version", "2", "--netns", "/var/run/netns/test"} {
		if !argSet[required] {
			t.Errorf("missing expected arg %q in %v", required, args)
		}
	}
}

func TestBuildArgs_NilConfig(t *testing.T) {
	var cfg *JailerConfig
	_, err := cfg.BuildArgs("/task", nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestBuildArgs_EmptyTaskDir(t *testing.T) {
	cfg := &JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer"}
	_, err := cfg.BuildArgs("", nil)
	if err == nil {
		t.Fatal("expected error for empty taskDir")
	}
}

func TestJailerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *JailerConfig
		wantErr bool
	}{
		{"nil config", nil, false},
		{"valid", &JailerConfig{ExecFile: "firecracker", JailerBinary: "jailer"}, false},
		{"missing exec_file", &JailerConfig{JailerBinary: "jailer"}, true},
		{"missing jailer_binary", &JailerConfig{ExecFile: "firecracker"}, true},
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

func TestBin(t *testing.T) {
	if got := (*JailerConfig)(nil).Bin(); got != "" {
		t.Errorf("nil.Bin() = %q, want \"\"", got)
	}
	cfg := &JailerConfig{JailerBinary: "jailer"}
	if got := cfg.Bin(); got != "jailer" {
		t.Errorf("Bin() = %q, want \"jailer\"", got)
	}
}
