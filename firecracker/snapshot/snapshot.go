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

// Loc identifies the snapshot storage location for a task. Construct once
// and call methods instead of passing 5 parameters to every function.
type Loc struct {
	BasePath  string // plugin-level snapshot_path (empty = ephemeral)
	TaskDir   string // Nomad task directory
	Namespace string // Nomad namespace (isolates cross-tenant snapshots)
	JobID     string // Nomad job ID (from the job stanza, not the display name)
	GroupName string
	TaskName  string
}

// Dir returns the directory where snapshot files are stored.
// If BasePath is set (persistent mode), files go under
// <BasePath>/<Namespace>/<JobID>/<GroupName>/<TaskName>/. Otherwise they go under
// <TaskDir>/snapshots/ (ephemeral, within-allocation only).
func (l Loc) Dir() string {
	if l.BasePath != "" {
		return filepath.Join(l.BasePath, l.Namespace, l.JobID, l.GroupName, l.TaskName)
	}
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
