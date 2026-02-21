// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package jailer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileLinkRequest describes a file to be hard-linked into the jailer chroot
type FileLinkRequest struct {
	// SourcePath is the absolute path to the source file on the host
	SourcePath string
	// TargetName is the filename (not path) to use inside the chroot
	TargetName string
}

// LinkFilesIntoJail creates hard links for guest files inside the jailer chroot.
// This follows the official Firecracker pattern: files must be hard-linked into the
// chroot directory because the jailed Firecracker process cannot access host paths.
//
// Returns a map of source paths to their target names in the chroot (for use in VM config).
func LinkFilesIntoJail(jailerRootPath string, requests []FileLinkRequest) (map[string]string, error) {
	if len(requests) == 0 {
		return make(map[string]string), nil
	}

	// Ensure root directory exists
	if err := os.MkdirAll(jailerRootPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create jailer root directory: %w", err)
	}

	linkedPaths := make(map[string]string)

	for _, req := range requests {
		// Validate SourcePath is not empty
		if req.SourcePath == "" {
			return nil, fmt.Errorf("file link request has empty SourcePath")
		}
		// Validate TargetName is not empty
		if req.TargetName == "" {
			return nil, fmt.Errorf("file link request for source %q has empty TargetName", req.SourcePath)
		}
		// Validate TargetName is just a filename, not a path
		if filepath.Base(req.TargetName) != req.TargetName {
			return nil, fmt.Errorf("file link request for source %q has invalid TargetName %q (must be a filename, not a path)", req.SourcePath, req.TargetName)
		}

		// Verify source file exists and is readable
		if _, err := os.Stat(req.SourcePath); err != nil {
			return nil, fmt.Errorf("source file not accessible: %s: %w", req.SourcePath, err)
		}

		targetPath := filepath.Join(jailerRootPath, req.TargetName)

		// Create hard link
		// Hard links provide secure isolation: the linked file cannot be followed outside the chroot
		// This is preferred over symlinks (which can be attacked) or copies (which waste space)
		if err := os.Link(req.SourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("failed to create hard link: %s -> %s: %w", req.SourcePath, targetPath, err)
		}

		// Store the target name (relative path as seen from inside chroot)
		linkedPaths[req.SourcePath] = req.TargetName
	}

	return linkedPaths, nil
}

// LinkGuestFilesRequest bundles host paths that need to be linked into the chroot
type LinkGuestFilesRequest struct {
	KernelImagePath string
	InitrdPath      string
	DrivePaths      []string
}

// LinkGuestFilesForTask orchestrates file linking for a task: creates hard links for
// kernel, initrd, and drives into the jailer chroot root, and returns the relative
// filenames to use in the VM config.
func LinkGuestFilesForTask(jailerRootPath string, req *LinkGuestFilesRequest) (map[string]string, error) {
	if req == nil {
		return make(map[string]string), nil
	}

	linkRequests := make([]FileLinkRequest, 0)

	// Queue kernel image
	if req.KernelImagePath != "" {
		linkRequests = append(linkRequests, FileLinkRequest{
			SourcePath: req.KernelImagePath,
			TargetName: filepath.Base(req.KernelImagePath),
		})
	}

	// Queue initrd if specified
	if req.InitrdPath != "" {
		linkRequests = append(linkRequests, FileLinkRequest{
			SourcePath: req.InitrdPath,
			TargetName: filepath.Base(req.InitrdPath),
		})
	}

	// Queue all drive images
	for _, drivePath := range req.DrivePaths {
		if drivePath != "" {
			linkRequests = append(linkRequests, FileLinkRequest{
				SourcePath: drivePath,
				TargetName: filepath.Base(drivePath),
			})
		}
	}

	// Detect collisions in target filenames before creating links so we can return
	// a clear validation error instead of a low-level "file exists" from os.Link
	nameToSources := make(map[string][]string, len(linkRequests))
	for _, lr := range linkRequests {
		if lr.TargetName == "" {
			continue
		}
		nameToSources[lr.TargetName] = append(nameToSources[lr.TargetName], lr.SourcePath)
	}
	for name, sources := range nameToSources {
		if len(sources) > 1 {
			return nil, fmt.Errorf("multiple guest files share same target name %q: %v", name, sources)
		}
	}

	// Link all files into chroot
	return LinkFilesIntoJail(jailerRootPath, linkRequests)
}

// isAllowedImagePath checks if a path is within the allocation directory or
// within the configured allowlist of image paths. This prevents tenants from
// specifying arbitrary host paths.
// Note: Symlink resolution is performed separately just before hard linking
// to prevent TOCTOU (Time-of-check-time-of-use) vulnerabilities.
func isAllowedImagePath(allowedPaths []string, allocDir, imagePath string) bool {
	if !filepath.IsAbs(imagePath) {
		imagePath = filepath.Join(allocDir, imagePath)
	}

	isParent := func(parent, path string) bool {
		rel, err := filepath.Rel(parent, path)
		return err == nil && !strings.HasPrefix(rel, "..")
	}

	// Check if path is under alloc dir
	if isParent(allocDir, imagePath) {
		return true
	}

	// Check allowed paths
	for _, ap := range allowedPaths {
		if isParent(ap, imagePath) {
			return true
		}
	}

	return false
}

// ValidateAndResolvePath validates, converts, and resolves a single guest file path.
// Handles: relative→absolute conversion, symlink resolution, and re-validation against allowed paths.
// Returns resolved path for use in LinkGuestFilesRequest, or error if validation fails.
func ValidateAndResolvePath(path, fieldName, allocDir string, allowedPaths []string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Convert relative→absolute
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(allocDir, absPath)
	}

	// Validate against allowed paths (catches obvious violations early)
	if !isAllowedImagePath(allowedPaths, allocDir, absPath) {
		return "", fmt.Errorf("%s %q is not in allowed paths", fieldName, path)
	}

	// Resolve symlinks (TOCTOU defense: deferred until just before linking)
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s symlink: %w", fieldName, err)
	}

	// Re-validate resolved target (symlink escape prevention)
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

// PrepareGuestFilesParams holds parameters for preparing guest files
type PrepareGuestFilesParams struct {
	Config       *GuestFileConfig
	AllocDir     string
	AllowedPaths []string
	ChrootPath   string
}

// PrepareGuestFiles orchestrates entire guest file preparation: validates paths,
// resolves symlinks, links files into chroot, and returns config with relative paths.
// This is the primary entry point for guest file handling from the driver package.
func PrepareGuestFiles(params *PrepareGuestFilesParams) error {
	if params == nil || params.Config == nil {
		return fmt.Errorf("prepare guest files: invalid parameters")
	}

	req := &LinkGuestFilesRequest{}
	resolvedPaths := make(map[string]string) // Maps original → resolved paths

	// Process kernel
	if params.Config.Kernel != "" {
		absPath := params.Config.Kernel
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(params.AllocDir, absPath)
		}
		resolved, err := ValidateAndResolvePath(absPath, "kernel", params.AllocDir, params.AllowedPaths)
		if err != nil {
			return err
		}
		resolvedPaths[params.Config.Kernel] = resolved
		req.KernelImagePath = resolved
	}

	// Process initrd
	if params.Config.Initrd != "" {
		absPath := params.Config.Initrd
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(params.AllocDir, absPath)
		}
		resolved, err := ValidateAndResolvePath(absPath, "initrd", params.AllocDir, params.AllowedPaths)
		if err != nil {
			return err
		}
		resolvedPaths[params.Config.Initrd] = resolved
		req.InitrdPath = resolved
	}

	// Process drives
	if len(params.Config.Drives) > 0 {
		req.DrivePaths = make([]string, len(params.Config.Drives))
		for i, drivePath := range params.Config.Drives {
			if drivePath != "" {
				absPath := drivePath
				if !filepath.IsAbs(absPath) {
					absPath = filepath.Join(params.AllocDir, absPath)
				}
				resolved, err := ValidateAndResolvePath(absPath, fmt.Sprintf("drive[%d]", i), params.AllocDir, params.AllowedPaths)
				if err != nil {
					return err
				}
				resolvedPaths[drivePath] = resolved
				req.DrivePaths[i] = resolved
			}
		}
	}

	// Link files into chroot and get relative paths
	linkedPaths, err := LinkGuestFilesForTask(params.ChrootPath, req)
	if err != nil {
		return fmt.Errorf("failed to link guest files into jailer chroot: %w", err)
	}

	// Update config with relative paths
	if params.Config.Kernel != "" {
		resolved := resolvedPaths[params.Config.Kernel]
		if resolved == "" {
			resolved = params.Config.Kernel
		}
		if relativeName, ok := linkedPaths[resolved]; ok {
			params.Config.Kernel = relativeName
		}
	}

	if params.Config.Initrd != "" {
		resolved := resolvedPaths[params.Config.Initrd]
		if resolved == "" {
			resolved = params.Config.Initrd
		}
		if relativeName, ok := linkedPaths[resolved]; ok {
			params.Config.Initrd = relativeName
		}
	}

	for i, drivePath := range params.Config.Drives {
		if drivePath != "" {
			resolved := resolvedPaths[drivePath]
			if resolved == "" {
				resolved = drivePath
			}
			if relativeName, ok := linkedPaths[resolved]; ok {
				params.Config.Drives[i] = relativeName
			}
		}
	}

	return nil
}
