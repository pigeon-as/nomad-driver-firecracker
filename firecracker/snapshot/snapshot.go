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

// Loc identifies the snapshot storage location for a task.
type Loc struct {
	TaskDir string // Nomad task directory (cfg.TaskDir().Dir)
}

// Dir returns the directory where snapshot files are stored:
// <TaskDir>/snapshots/. The task directory is scoped to the allocation,
// so snapshots naturally survive alloc restarts (autostop/autostart) but
// are discarded when a new allocation is created (job update, reschedule).
func (l Loc) Dir() string {
	return filepath.Join(l.TaskDir, snapshotDirName)
}

// Has reports whether both snapshot files exist in the snapshot directory.
func (l Loc) Has() bool {
	dir := l.Dir()
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
func (l Loc) Save(chrootRoot string) error {
	dir := l.Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}
	for _, name := range []string{VMStateName, MemName} {
		src := filepath.Join(chrootRoot, name)
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
func (l Loc) Link(chrootRoot string) error {
	dir := l.Dir()
	for _, name := range []string{VMStateName, MemName} {
		src := filepath.Join(dir, name)
		dst := filepath.Join(chrootRoot, name)
		if err := os.Link(src, dst); err != nil {
			return fmt.Errorf("link snapshot file %s: %w", name, err)
		}
	}
	return nil
}

// RemoveDir removes the snapshot directory. Used when a snapshot restore
// fails to ensure the next start falls back to cold boot.
func (l Loc) RemoveDir() error {
	return os.RemoveAll(l.Dir())
}
