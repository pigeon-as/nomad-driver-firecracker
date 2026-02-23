// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	snapshotDirName = "snapshots"
	// File names for snapshot artifacts inside the snapshot directory and chroot.
	VMStateName = "vmstate"
	MemName     = "memory"
	// Chroot-relative paths used in Firecracker API calls.
	VMStatePath = "/" + VMStateName
	MemPath     = "/" + MemName
)

// Dir returns the directory where snapshot files are stored.
// If snapshotPath is set (persistent mode), files go under
// <snapshotPath>/<jobID>/<taskName>/. Otherwise they go under
// <taskDir>/snapshots/ (ephemeral, within-allocation only).
func Dir(snapshotPath, taskDir, jobID, taskName string) string {
	if snapshotPath != "" {
		return filepath.Join(snapshotPath, jobID, taskName)
	}
	return filepath.Join(taskDir, snapshotDirName)
}

// Has reports whether both snapshot files exist in the snapshot directory.
func Has(snapshotPath, taskDir, jobID, taskName string) bool {
	dir := Dir(snapshotPath, taskDir, jobID, taskName)
	for _, name := range []string{VMStateName, MemName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

// Save moves snapshot artifacts from the chroot root to the snapshot
// directory. This must be called before DestroyTask cleans the chroot.
// Both directories must be on the same filesystem (rename is an instant
// metadata operation; cross-device moves would require copying the full
// memory file and are not supported).
func Save(chrootRootPath, snapshotPath, taskDir, jobID, taskName string) error {
	dir := Dir(snapshotPath, taskDir, jobID, taskName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}
	for _, name := range []string{VMStateName, MemName} {
		src := filepath.Join(chrootRootPath, name)
		dst := filepath.Join(dir, name)
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("move snapshot file %s: %w", name, err)
		}
	}
	return nil
}

// Link hard-links snapshot artifacts from the snapshot directory into the
// chroot root so Firecracker can load them. Both directories must be on
// the same filesystem.
func Link(snapshotPath, taskDir, jobID, taskName, chrootRootPath string) error {
	dir := Dir(snapshotPath, taskDir, jobID, taskName)
	for _, name := range []string{VMStateName, MemName} {
		src := filepath.Join(dir, name)
		dst := filepath.Join(chrootRootPath, name)
		if err := os.Link(src, dst); err != nil {
			return fmt.Errorf("link snapshot file %s: %w", name, err)
		}
	}
	return nil
}

// RemoveDir removes the snapshot directory. Used when a snapshot restore
// fails to ensure the next start falls back to cold boot.
func RemoveDir(snapshotPath, taskDir, jobID, taskName string) error {
	return os.RemoveAll(Dir(snapshotPath, taskDir, jobID, taskName))
}
