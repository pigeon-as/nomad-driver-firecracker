// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package jailer

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	snapshotDirName = "snapshots"
	// File names for snapshot artifacts inside the snapshot directory and chroot.
	SnapshotVMStateName = "vmstate"
	SnapshotMemName     = "memory"
	// Chroot-relative paths used in Firecracker API calls.
	SnapshotVMStatePath = "/" + SnapshotVMStateName
	SnapshotMemPath     = "/" + SnapshotMemName
)

// SnapshotDir returns <task_dir>/snapshots, which lives outside the jailer
// chroot and survives DestroyTask cleanup.
func SnapshotDir(taskDir string) string {
	return filepath.Join(taskDir, snapshotDirName)
}

// HasSnapshot reports whether both snapshot files exist in the snapshot directory.
func HasSnapshot(taskDir string) bool {
	dir := SnapshotDir(taskDir)
	for _, name := range []string{SnapshotVMStateName, SnapshotMemName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

// SaveSnapshotFiles moves snapshot artifacts from the chroot root to the
// snapshot directory. This must be called before DestroyTask cleans the chroot.
// Both directories must be on the same filesystem (rename is an instant
// metadata operation; cross-device moves would require copying the full
// memory file and are not supported).
func SaveSnapshotFiles(chrootRootPath, taskDir string) error {
	dir := SnapshotDir(taskDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}
	for _, name := range []string{SnapshotVMStateName, SnapshotMemName} {
		src := filepath.Join(chrootRootPath, name)
		dst := filepath.Join(dir, name)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("move snapshot file %s: %w", name, err)
		}
	}
	return nil
}

// LinkSnapshotFiles hard-links snapshot artifacts from the snapshot directory
// into the chroot root so Firecracker can load them. Both directories must
// be on the same filesystem.
func LinkSnapshotFiles(taskDir, chrootRootPath string) error {
	dir := SnapshotDir(taskDir)
	for _, name := range []string{SnapshotVMStateName, SnapshotMemName} {
		src := filepath.Join(dir, name)
		dst := filepath.Join(chrootRootPath, name)
		if err := os.Link(src, dst); err != nil {
			return fmt.Errorf("link snapshot file %s: %w", name, err)
		}
	}
	return nil
}
