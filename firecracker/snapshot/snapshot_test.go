package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHas(t *testing.T) {
	taskDir := t.TempDir()
	loc := Loc{TaskDir: taskDir}

	if loc.Has() {
		t.Fatal("expected false with no snapshot dir")
	}

	dir := loc.Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Only one file present.
	if err := os.WriteFile(filepath.Join(dir, VMStateName), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if loc.Has() {
		t.Fatal("expected false with only vmstate")
	}

	// Both files present.
	if err := os.WriteFile(filepath.Join(dir, MemName), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if !loc.Has() {
		t.Fatal("expected true with both files")
	}
}

func TestSaveAndLink(t *testing.T) {
	chroot := t.TempDir()
	taskDir := t.TempDir()
	loc := Loc{TaskDir: taskDir}

	// Create fake snapshot files in chroot.
	if err := os.WriteFile(filepath.Join(chroot, VMStateName), []byte("state"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chroot, MemName), []byte("mem"), 0600); err != nil {
		t.Fatal(err)
	}

	// Save (move) them to taskDir.
	if err := loc.Save(chroot); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !loc.Has() {
		t.Fatal("snapshot should exist after save")
	}
	// Source files should be gone.
	if _, err := os.Stat(filepath.Join(chroot, VMStateName)); !os.IsNotExist(err) {
		t.Fatal("source vmstate should have been moved")
	}

	// Link them back into a new chroot.
	newChroot := t.TempDir()
	if err := loc.Link(newChroot); err != nil {
		t.Fatalf("Link: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(newChroot, VMStateName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "state" {
		t.Fatalf("linked vmstate = %q, want %q", data, "state")
	}
}

func TestSave_MissingSource(t *testing.T) {
	chroot := t.TempDir()
	taskDir := t.TempDir()
	loc := Loc{TaskDir: taskDir}
	if err := loc.Save(chroot); err == nil {
		t.Fatal("expected error for missing source files")
	}
}

func TestDir(t *testing.T) {
	loc := Loc{TaskDir: "/alloc/task"}
	got := loc.Dir()
	want := filepath.Join("/alloc/task", "snapshots")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}
