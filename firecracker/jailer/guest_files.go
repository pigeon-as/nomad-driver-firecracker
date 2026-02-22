// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package jailer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// LinkGuestFiles links kernel, initrd, and drives into chroot by their basename.
// If multiple files share the same basename, returns an error.
func LinkGuestFiles(jailerRootPath string, kernelPath, initrdPath string, drivePaths []string) error {
	if jailerRootPath == "" {
		return fmt.Errorf("jailer root path cannot be empty")
	}

	// Ensure root directory exists
	if err := os.MkdirAll(jailerRootPath, 0750); err != nil {
		return fmt.Errorf("failed to create jailer root directory: %w", err)
	}

	// Collect all files to link
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

		// Make linking idempotent: handle existing target paths.
		if targetInfo, err := os.Lstat(targetPath); err == nil {
			// Target exists; verify it already points to the same file.
			srcInfo, srcErr := os.Stat(sourcePath)
			if srcErr != nil {
				return fmt.Errorf("failed to stat source file %s: %w", sourcePath, srcErr)
			}
			if os.SameFile(srcInfo, targetInfo) {
				// Already correctly linked; nothing to do.
				continue
			}
			// Conflicting existing file; remove and recreate the link.
			if rmErr := os.Remove(targetPath); rmErr != nil {
				return fmt.Errorf("failed to remove existing target %s: %w", targetPath, rmErr)
			}
		} else if !os.IsNotExist(err) {
			// An unexpected error occurred while checking the target.
			return fmt.Errorf("failed to stat target %s: %w", targetPath, err)
		}

		if err := os.Link(sourcePath, targetPath); err != nil {
			if errors.Is(err, syscall.EXDEV) {
				if copyErr := copyFile(sourcePath, targetPath); copyErr != nil {
					return fmt.Errorf("failed to copy %s -> %s: %w", sourcePath, targetPath, copyErr)
				}
				continue
			}
			return fmt.Errorf("failed to hard link %s -> %s: %w", sourcePath, targetPath, err)
		}
	}

	return nil
}

func copyFile(sourcePath, targetPath string) error {
	srcFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
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

// ValidateAndResolvePath validates and resolves a guest file path.
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

// GuestFileConfig represents the guest file configuration to be prepared
type GuestFileConfig struct {
	Kernel string
	Initrd string
	Drives []string
}

// GuestFilePaths holds the resolved relative paths for linked guest files
type GuestFilePaths struct {
	Kernel string
	Initrd string
	Drives []string
}

// PrepareGuestFilesParams holds parameters for preparing guest files
type PrepareGuestFilesParams struct {
	Config       *GuestFileConfig
	AllocDir     string
	AllowedPaths []string
	ChrootPath   string
}

// PrepareGuestFiles validates, resolves, and links guest files into chroot.
// Returns the resolved relative basenames in GuestFilePaths.
// The input config is not modified.
func PrepareGuestFiles(params *PrepareGuestFilesParams) (*GuestFilePaths, error) {
	if params == nil || params.Config == nil {
		return nil, fmt.Errorf("prepare guest files: invalid parameters")
	}

	// Validate and resolve kernel path
	var kernelPath string
	if params.Config.Kernel != "" {
		var err error
		kernelPath, err = ValidateAndResolvePath(params.Config.Kernel, "kernel", params.AllocDir, params.AllowedPaths)
		if err != nil {
			return nil, err
		}
	}

	// Validate and resolve initrd path
	var initrdPath string
	if params.Config.Initrd != "" {
		var err error
		initrdPath, err = ValidateAndResolvePath(params.Config.Initrd, "initrd", params.AllocDir, params.AllowedPaths)
		if err != nil {
			return nil, err
		}
	}

	// Validate and resolve drive paths
	var drivePaths []string
	if len(params.Config.Drives) > 0 {
		drivePaths = make([]string, len(params.Config.Drives))
		for i, drivePathCfg := range params.Config.Drives {
			if drivePathCfg != "" {
				var err error
				drivePaths[i], err = ValidateAndResolvePath(drivePathCfg, fmt.Sprintf("drive[%d]", i), params.AllocDir, params.AllowedPaths)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// Link all files into chroot
	if err := LinkGuestFiles(params.ChrootPath, kernelPath, initrdPath, drivePaths); err != nil {
		return nil, fmt.Errorf("failed to link guest files: %w", err)
	}

	// Build result with relative basenames - only call Base() when paths are non-empty
	// (filepath.Base("") returns ".", which would incorrectly set InitrdPath=".")
	var kernelBase, initrdBase string
	if kernelPath != "" {
		kernelBase = filepath.Base(kernelPath)
	}
	if initrdPath != "" {
		initrdBase = filepath.Base(initrdPath)
	}

	paths := &GuestFilePaths{
		Kernel: kernelBase,
		Initrd: initrdBase,
	}
	if len(drivePaths) > 0 {
		paths.Drives = make([]string, len(drivePaths))
		for i, p := range drivePaths {
			if p != "" {
				paths.Drives[i] = filepath.Base(p)
			}
		}
	}

	return paths, nil
}
