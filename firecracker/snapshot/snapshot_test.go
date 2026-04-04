package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"
)

func TestHas(t *testing.T) {
	taskDir := t.TempDir()
	loc := Loc{TaskDir: taskDir}

	must.False(t, loc.Has())

	dir := loc.Dir()
	must.NoError(t, os.MkdirAll(dir, 0700))
	// Only one file present.
	must.NoError(t, os.WriteFile(filepath.Join(dir, VMStateName), []byte("x"), 0600))
	must.False(t, loc.Has())

	// Both files present.
	must.NoError(t, os.WriteFile(filepath.Join(dir, MemName), []byte("x"), 0600))
	must.True(t, loc.Has())
}

func TestSaveAndLink(t *testing.T) {
	chroot := t.TempDir()
	taskDir := t.TempDir()
	loc := Loc{TaskDir: taskDir}

	// Create fake snapshot files in chroot.
	must.NoError(t, os.WriteFile(filepath.Join(chroot, VMStateName), []byte("state"), 0600))
	must.NoError(t, os.WriteFile(filepath.Join(chroot, MemName), []byte("mem"), 0600))

	// Save (move) them to taskDir.
	must.NoError(t, loc.Save(chroot))
	must.True(t, loc.Has())
	// Source files should be gone.
	_, err := os.Stat(filepath.Join(chroot, VMStateName))
	must.True(t, os.IsNotExist(err))

	// Link them back into a new chroot.
	newChroot := t.TempDir()
	must.NoError(t, loc.Link(newChroot))
	data, err := os.ReadFile(filepath.Join(newChroot, VMStateName))
	must.NoError(t, err)
	must.EqOp(t, "state", string(data))
}

func TestSave_MissingSource(t *testing.T) {
	chroot := t.TempDir()
	taskDir := t.TempDir()
	loc := Loc{TaskDir: taskDir}
	must.Error(t, loc.Save(chroot))
}
