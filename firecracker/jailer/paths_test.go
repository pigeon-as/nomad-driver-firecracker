package jailer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"
)

func TestBuildChrootDir(t *testing.T) {
	tmp := t.TempDir()

	chrootRoot, err := BuildChrootDir(tmp, "task-1", "firecracker")
	must.NoError(t, err)

	root := filepath.Join(tmp, "firecracker", "task-1", "root")
	_, err = os.Stat(root)
	must.NoError(t, err)
	must.EqOp(t, root, chrootRoot)
}

func TestSocketPathRoundtrip(t *testing.T) {
	jailerDir := "/srv/jailer/firecracker/task-1"
	sock := SocketPath(jailerDir)
	got := TaskDirFromSocketPath(sock)
	must.EqOp(t, jailerDir, got)
}

func TestFindTaskDir(t *testing.T) {
	tmp := t.TempDir()

	// No match
	dir, err := FindTaskDir(tmp, "nonexistent")
	must.NoError(t, err)
	must.EqOp(t, "", dir)

	// Single match
	taskDir := filepath.Join(tmp, "firecracker", "task-1")
	must.NoError(t, os.MkdirAll(taskDir, 0700))
	dir, err = FindTaskDir(tmp, "task-1")
	must.NoError(t, err)
	must.EqOp(t, taskDir, dir)

	// Multiple matches → error
	dup := filepath.Join(tmp, "other-binary", "task-1")
	must.NoError(t, os.MkdirAll(dup, 0700))
	_, err = FindTaskDir(tmp, "task-1")
	must.Error(t, err)
}

func TestFindAllTaskDirs(t *testing.T) {
	tmp := t.TempDir()

	dir1 := filepath.Join(tmp, "firecracker", "task-1")
	dir2 := filepath.Join(tmp, "other", "task-1")
	for _, d := range []string{dir1, dir2} {
		must.NoError(t, os.MkdirAll(d, 0700))
	}

	dirs, err := FindAllTaskDirs(tmp, "task-1")
	must.NoError(t, err)
	must.SliceLen(t, 2, dirs)
}

func TestValidateSocketPath(t *testing.T) {
	// Short path should pass.
	must.NoError(t, ValidateSocketPath("/srv/jailer", "task-1", "firecracker"))

	// Path exceeding 107 bytes should fail.
	longID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	must.Error(t, ValidateSocketPath("/srv/jailer", longID, "firecracker"))
}
