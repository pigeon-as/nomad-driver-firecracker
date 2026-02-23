package jailer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasSnapshot(t *testing.T) {
	taskDir := t.TempDir()
	if HasSnapshot(taskDir) {
		t.Fatal("expected false with no snapshot dir")
	}

	dir := SnapshotDir(taskDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Only one file present.
	if err := os.WriteFile(filepath.Join(dir, SnapshotVMStateName), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if HasSnapshot(taskDir) {
		t.Fatal("expected false with only vmstate")
	}

	// Both files present.
	if err := os.WriteFile(filepath.Join(dir, SnapshotMemName), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if !HasSnapshot(taskDir) {
		t.Fatal("expected true with both files")
	}
}

func TestSaveAndLinkSnapshotFiles(t *testing.T) {
	chroot := t.TempDir()
	taskDir := t.TempDir()

	// Create fake snapshot files in chroot.
	if err := os.WriteFile(filepath.Join(chroot, SnapshotVMStateName), []byte("state"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chroot, SnapshotMemName), []byte("mem"), 0600); err != nil {
		t.Fatal(err)
	}

	// Save (move) them to taskDir.
	if err := SaveSnapshotFiles(chroot, taskDir); err != nil {
		t.Fatalf("SaveSnapshotFiles: %v", err)
	}
	if !HasSnapshot(taskDir) {
		t.Fatal("snapshot should exist after save")
	}
	// Source files should be gone.
	if _, err := os.Stat(filepath.Join(chroot, SnapshotVMStateName)); !os.IsNotExist(err) {
		t.Fatal("source vmstate should have been moved")
	}

	// Link them back into a new chroot.
	newChroot := t.TempDir()
	if err := LinkSnapshotFiles(taskDir, newChroot); err != nil {
		t.Fatalf("LinkSnapshotFiles: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(newChroot, SnapshotVMStateName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "state" {
		t.Fatalf("linked vmstate = %q, want %q", data, "state")
	}
}

func TestSaveSnapshotFiles_MissingSource(t *testing.T) {
	chroot := t.TempDir()
	taskDir := t.TempDir()
	if err := SaveSnapshotFiles(chroot, taskDir); err == nil {
		t.Fatal("expected error for missing source files")
	}
}
