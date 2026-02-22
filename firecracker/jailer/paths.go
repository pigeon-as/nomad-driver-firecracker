package jailer

import (
	"os"
	"path/filepath"
)

// Paths holds jailer and chroot paths used by the driver.
type Paths struct {
	ConfigPathHost   string
	ConfigPathChroot string
}

// BuildPaths prepares jailer paths under the task directory and ensures the chroot root exists.
// The path follows the Firecracker jailer layout: <chroot_base>/<exec_file_name>/<id>/root
func BuildPaths(taskDir, taskID, execFile string) (*Paths, error) {
	execFileName := filepath.Base(execFile)
	root := filepath.Join(taskDir, "jailer", execFileName, taskID, "root")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}

	return &Paths{
		ConfigPathHost:   filepath.Join(root, "vmconfig.json"),
		ConfigPathChroot: "/vmconfig.json",
	}, nil
}
