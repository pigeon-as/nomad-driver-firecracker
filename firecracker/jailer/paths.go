package jailer

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths holds jailer and chroot paths used by the driver.
type Paths struct {
	ConfigPathHost   string
	ConfigPathChroot string
}

const chrootBaseDirName = "jailer"

// ChrootBaseDir returns <task_dir>/jailer.
func ChrootBaseDir(taskDir string) string {
	return filepath.Join(taskDir, chrootBaseDirName)
}

// TaskDir returns <task_dir>/jailer/<exec_file_name>/<task_id>.
func TaskDir(taskDir, taskID, execFile string) string {
	return filepath.Join(ChrootBaseDir(taskDir), filepath.Base(execFile), taskID)
}

// BuildPaths creates the jailer chroot directory and returns config file paths.
func BuildPaths(taskDir, taskID, execFile string) (*Paths, error) {
	root := filepath.Join(TaskDir(taskDir, taskID, execFile), "root")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}

	return &Paths{
		ConfigPathHost:   filepath.Join(root, "vmconfig.json"),
		ConfigPathChroot: "/vmconfig.json",
	}, nil
}

// FindTaskDir discovers <task_dir>/jailer/*/<task_id> via glob.
func FindTaskDir(taskDir, taskID string) (string, error) {
	pattern := filepath.Join(ChrootBaseDir(taskDir), "*", taskID)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid jailer path pattern %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple jailer directories found for task %q", taskID)
	}
	return matches[0], nil
}

// SocketPath returns the Firecracker API socket path within a task jailer directory.
func SocketPath(taskJailerDir string) string {
	if taskJailerDir == "" {
		return ""
	}
	return filepath.Join(taskJailerDir, "root", "run", "firecracker.socket")
}

// FindTaskSocketPath discovers the task jailer directory and returns its socket path.
func FindTaskSocketPath(taskDir, taskID string) (string, error) {
	taskJailerDir, err := FindTaskDir(taskDir, taskID)
	if err != nil || taskJailerDir == "" {
		return "", err
	}
	return SocketPath(taskJailerDir), nil
}

// TaskDirFromSocketPath derives the task jailer directory from a socket path.
func TaskDirFromSocketPath(socketPath string) string {
	if socketPath == "" {
		return ""
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(socketPath)))
}
