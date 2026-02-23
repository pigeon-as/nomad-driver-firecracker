package jailer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskDir(t *testing.T) {
	got := TaskDir("/srv/jailer", "abc-123", "/usr/bin/firecracker")
	want := filepath.Join("/srv/jailer", "firecracker", "abc-123")
	if got != want {
		t.Errorf("TaskDir = %q, want %q", got, want)
	}
}

func TestBuildChrootDir(t *testing.T) {
	tmp := t.TempDir()

	chrootRoot, err := BuildChrootDir(tmp, "task-1", "firecracker")
	if err != nil {
		t.Fatalf("BuildChrootDir: %v", err)
	}

	root := filepath.Join(tmp, "firecracker", "task-1", "root")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("root dir not created: %v", err)
	}
	if chrootRoot != root {
		t.Errorf("ChrootRoot = %q, want %q", chrootRoot, root)
	}
}

func TestSocketPathRoundtrip(t *testing.T) {
	jailerDir := "/srv/jailer/firecracker/task-1"
	sock := SocketPath(jailerDir)
	got := TaskDirFromSocketPath(sock)
	if got != jailerDir {
		t.Errorf("TaskDirFromSocketPath(SocketPath(%q)) = %q, want %q", jailerDir, got, jailerDir)
	}
}

func TestSocketPath_Empty(t *testing.T) {
	if got := SocketPath(""); got != "" {
		t.Errorf("SocketPath(\"\") = %q, want \"\"", got)
	}
	if got := TaskDirFromSocketPath(""); got != "" {
		t.Errorf("TaskDirFromSocketPath(\"\") = %q, want \"\"", got)
	}
}

func TestFindTaskDir(t *testing.T) {
	tmp := t.TempDir()

	// No match
	dir, err := FindTaskDir(tmp, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "" {
		t.Errorf("expected empty, got %q", dir)
	}

	// Single match
	taskDir := filepath.Join(tmp, "firecracker", "task-1")
	if err := os.MkdirAll(taskDir, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err = FindTaskDir(tmp, "task-1")
	if err != nil {
		t.Fatalf("FindTaskDir: %v", err)
	}
	if dir != taskDir {
		t.Errorf("FindTaskDir = %q, want %q", dir, taskDir)
	}

	// Multiple matches → error
	dup := filepath.Join(tmp, "other-binary", "task-1")
	if err := os.MkdirAll(dup, 0700); err != nil {
		t.Fatal(err)
	}
	_, err = FindTaskDir(tmp, "task-1")
	if err == nil {
		t.Fatal("expected error for multiple matches")
	}
}

func TestFindAllTaskDirs(t *testing.T) {
	tmp := t.TempDir()

	dir1 := filepath.Join(tmp, "firecracker", "task-1")
	dir2 := filepath.Join(tmp, "other", "task-1")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}

	dirs, err := FindAllTaskDirs(tmp, "task-1")
	if err != nil {
		t.Fatalf("FindAllTaskDirs: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}
}

func TestValidateSocketPath(t *testing.T) {
	// Short path should pass.
	if err := ValidateSocketPath("/srv/jailer", "task-1", "firecracker"); err != nil {
		t.Fatalf("expected valid: %v", err)
	}

	// Path exceeding 107 bytes should fail.
	longID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := ValidateSocketPath("/srv/jailer", longID, "firecracker"); err == nil {
		t.Fatal("expected error for long socket path")
	}
}
