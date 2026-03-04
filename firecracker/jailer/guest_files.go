// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package jailer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// LinkGuestFiles links kernel, initrd, and drives into chroot by their basename.
// This is intended for regular files (kernel images, initrd, disk images).
// For block devices (e.g. LVM volumes), use LinkDeviceNodes instead.
func LinkGuestFiles(jailerRootPath string, kernelPath, initrdPath string, drivePaths []string) error {
	if jailerRootPath == "" {
		return fmt.Errorf("jailer root path cannot be empty")
	}

	// Ensure root directory exists
	if err := os.MkdirAll(jailerRootPath, 0750); err != nil {
		return fmt.Errorf("failed to create jailer root directory: %w", err)
	}

	files := make(map[string]string) // targetName -> sourcePath

	if kernelPath != "" {
		name := filepath.Base(kernelPath)
		if existing, ok := files[name]; ok && existing != kernelPath {
			return fmt.Errorf("multiple guest files share target name %q", name)
		}
		files[name] = kernelPath
	}

	if initrdPath != "" {
		name := filepath.Base(initrdPath)
		if existing, ok := files[name]; ok && existing != initrdPath {
			return fmt.Errorf("multiple guest files share target name %q", name)
		}
		files[name] = initrdPath
	}

	for _, drivePath := range drivePaths {
		if drivePath != "" {
			name := filepath.Base(drivePath)
			if existing, ok := files[name]; ok && existing != drivePath {
				return fmt.Errorf("multiple guest files share target name %q", name)
			}
			files[name] = drivePath
		}
	}
	// Link all files
	for targetName, sourcePath := range files {
		if _, err := os.Stat(sourcePath); err != nil {
			return fmt.Errorf("source file not accessible: %s: %w", sourcePath, err)
		}

		targetPath := filepath.Join(jailerRootPath, targetName)

		// Make linking idempotent.
		if targetInfo, err := os.Lstat(targetPath); err == nil {
			// Target exists; verify it already points to the same file.
			srcInfo, srcErr := os.Stat(sourcePath)
			if srcErr != nil {
				return fmt.Errorf("failed to stat source file %s: %w", sourcePath, srcErr)
			}
			if os.SameFile(srcInfo, targetInfo) {
				continue
			}
			if rmErr := os.Remove(targetPath); rmErr != nil {
				return fmt.Errorf("failed to remove existing target %s: %w", targetPath, rmErr)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat target %s: %w", targetPath, err)
		}

		if err := os.Link(sourcePath, targetPath); err != nil {
			if errors.Is(err, syscall.EXDEV) {
				return fmt.Errorf("cannot link %s -> %s: source and chroot are on different filesystems (EXDEV)", sourcePath, targetPath)
			}
			return fmt.Errorf("failed to hard link %s -> %s: %w", sourcePath, targetPath, err)
		}
	}

	return nil
}

// LinkDeviceNodes creates device nodes in chrootPath for block devices.
// Symlinks (e.g. /dev/vg/lv → /dev/dm-X) are resolved before
// reading major:minor numbers so mknod targets the real device.
// Returns the resolved paths (parallel to the input slice) so callers
// can update drive configs with the basenames.
func LinkDeviceNodes(chrootPath string, devicePaths []string) ([]string, error) {
	if chrootPath == "" {
		return nil, fmt.Errorf("chroot path cannot be empty")
	}

	if err := os.MkdirAll(chrootPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create chroot directory: %w", err)
	}

	resolved := make([]string, len(devicePaths))

	for i, devPath := range devicePaths {
		if devPath == "" {
			continue
		}

		// Resolve symlinks so we pick up the real major:minor.
		real, err := filepath.EvalSymlinks(devPath)
		if err != nil {
			return nil, fmt.Errorf("resolve symlink %s: %w", devPath, err)
		}
		resolved[i] = real

		targetPath := filepath.Join(chrootPath, filepath.Base(real))

		// Idempotent: skip if the target already exists as a device node
		// with the same major:minor.
		if targetInfo, stErr := os.Lstat(targetPath); stErr == nil {
			if targetInfo.Mode()&os.ModeDevice != 0 {
				var srcStat, tgtStat unix.Stat_t
				if unix.Stat(real, &srcStat) == nil &&
					unix.Stat(targetPath, &tgtStat) == nil &&
					srcStat.Rdev == tgtStat.Rdev {
					continue
				}
			}
			// Stale or non-device entry — remove and recreate.
			if rmErr := os.Remove(targetPath); rmErr != nil {
				return nil, fmt.Errorf("failed to remove existing target %s: %w", targetPath, rmErr)
			}
		} else if !os.IsNotExist(stErr) {
			return nil, fmt.Errorf("failed to stat target %s: %w", targetPath, stErr)
		}

		if err := MknodFromSource(real, targetPath); err != nil {
			return nil, fmt.Errorf("mknod for %s: %w", devPath, err)
		}
	}

	return resolved, nil
}

// MknodFromSource creates a device node at targetPath with the same type
// and major/minor numbers as sourcePath. Used for block devices (e.g. LVM
// logical volumes) which cannot be hard-linked (Linux returns EPERM).
func MknodFromSource(sourcePath, targetPath string) error {
	var stat unix.Stat_t
	if err := unix.Stat(sourcePath, &stat); err != nil {
		return fmt.Errorf("stat %s: %w", sourcePath, err)
	}

	mode := stat.Mode & unix.S_IFMT
	if mode != unix.S_IFBLK && mode != unix.S_IFCHR {
		return fmt.Errorf("%s is not a device node (mode 0x%x)", sourcePath, stat.Mode)
	}

	// Preserve device type (block/char) with 0660 permissions.
	if err := unix.Mknod(targetPath, mode|0660, int(stat.Rdev)); err != nil {
		return fmt.Errorf("mknod %s: %w", targetPath, err)
	}
	return nil
}

// isAllowedImagePath reports whether path is within allocDir or allowedPaths.
func isAllowedImagePath(allowedPaths []string, allocDir, imagePath string) bool {
	if !filepath.IsAbs(imagePath) {
		imagePath = filepath.Join(allocDir, imagePath)
	}

	isParent := func(parent, path string) bool {
		rel, err := filepath.Rel(parent, path)
		if err != nil {
			return false
		}
		// Reject only actual parent references (".." or "../...")
		return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
	}

	if isParent(allocDir, imagePath) {
		return true
	}
	for _, ap := range allowedPaths {
		if isParent(ap, imagePath) {
			return true
		}
	}

	return false
}

// ValidateAndResolvePath resolves a guest file path and validates it against allowed paths.
func ValidateAndResolvePath(path, fieldName, allocDir string, allowedPaths []string) (string, error) {
	if path == "" {
		return "", nil
	}

	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(allocDir, absPath)
	}

	if !isAllowedImagePath(allowedPaths, allocDir, absPath) {
		return "", fmt.Errorf("%s %q is not in allowed paths", fieldName, path)
	}

	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s symlink: %w", fieldName, err)
	}

	if !isAllowedImagePath(allowedPaths, allocDir, resolved) {
		return "", fmt.Errorf("%s symlink target %q is not in allowed paths", fieldName, resolved)
	}

	return resolved, nil
}

// PrepareGuestFiles validates, resolves, and links kernel/initrd/drive files
// into the jailer chroot. Returns the resolved absolute paths (empty string
// for optional unset paths). The caller should update its config to use
// filepath.Base() of each returned path for chroot-relative references.
func PrepareGuestFiles(chrootRoot, kernelPath, initrdPath string, drivePaths []string, allocDir string, allowedPaths []string) (resolvedKernel, resolvedInitrd string, resolvedDrives []string, err error) {
	resolvedKernel, err = ValidateAndResolvePath(kernelPath, "kernel", allocDir, allowedPaths)
	if err != nil {
		return
	}
	resolvedInitrd, err = ValidateAndResolvePath(initrdPath, "initrd", allocDir, allowedPaths)
	if err != nil {
		return
	}
	resolvedDrives = make([]string, len(drivePaths))
	for i, p := range drivePaths {
		resolvedDrives[i], err = ValidateAndResolvePath(p, fmt.Sprintf("drive[%d]", i), allocDir, allowedPaths)
		if err != nil {
			return
		}
	}
	if err = LinkGuestFiles(chrootRoot, resolvedKernel, resolvedInitrd, resolvedDrives); err != nil {
		err = fmt.Errorf("failed to link guest files: %w", err)
	}
	return
}