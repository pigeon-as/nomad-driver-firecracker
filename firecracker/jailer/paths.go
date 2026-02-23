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

// unixPathMax is the maximum usable length for a Unix domain socket path.
// Linux's struct sockaddr_un.sun_path is 108 bytes; one is consumed by
// the NUL terminator, leaving 107 usable characters.
const unixPathMax = 107

// TaskDir returns <chrootBase>/<exec_file_name>/<task_id>.
func TaskDir(chrootBase, taskID, execFile string) string {
	return filepath.Join(chrootBase, filepath.Base(execFile), taskID)
}

// BuildPaths creates the jailer chroot directory and returns config file paths.
func BuildPaths(chrootBase, taskID, execFile string) (*Paths, error) {
	root := filepath.Join(TaskDir(chrootBase, taskID, execFile), "root")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}

	return &Paths{
		ConfigPathHost:   filepath.Join(root, "vmconfig.json"),
		ConfigPathChroot: "/vmconfig.json",
	}, nil
}

// FindAllTaskDirs discovers all <chrootBase>/*/<task_id> directories via glob.
func FindAllTaskDirs(chrootBase, taskID string) ([]string, error) {
	pattern := filepath.Join(chrootBase, "*", taskID)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid jailer path pattern %q: %w", pattern, err)
	}
	return matches, nil
}

// FindTaskDir discovers a single <chrootBase>/*/<task_id> via glob.
// Returns an error if multiple directories match.
func FindTaskDir(chrootBase, taskID string) (string, error) {
	matches, err := FindAllTaskDirs(chrootBase, taskID)
	if err != nil {
		return "", err
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
func FindTaskSocketPath(chrootBase, taskID string) (string, error) {
	taskJailerDir, err := FindTaskDir(chrootBase, taskID)
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

// ValidateSocketPath checks that the resulting socket path will be within
// the Unix domain socket sun_path limit (107 usable bytes). Call this
// before creating any jailer directories.
func ValidateSocketPath(chrootBase, taskID, execFile string) error {
	p := SocketPath(TaskDir(chrootBase, taskID, execFile))
	if len(p) > unixPathMax {
		return fmt.Errorf(
			"socket path too long (%d > %d bytes): %s; "+
				"set a shorter chroot_base in plugin config",
			len(p), unixPathMax, p)
	}
	return nil
}
