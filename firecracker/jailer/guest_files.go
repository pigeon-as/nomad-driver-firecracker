// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package jailer

import (
	"fmt"
	"os"
	"path/filepath"
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
// Returns a map of target names (for use in VM config) to their linked paths.
func LinkFilesIntoJail(jailorRootPath string, requests []FileLinkRequest) (map[string]string, error) {
	if len(requests) == 0 {
		return make(map[string]string), nil
	}

	// Ensure root directory exists
	if err := os.MkdirAll(jailorRootPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create jailer root directory: %w", err)
	}

	linkedPaths := make(map[string]string)

	for _, req := range requests {
		if req.SourcePath == "" || req.TargetName == "" {
			continue
		}

		// Verify source file exists and is readable
		if _, err := os.Stat(req.SourcePath); err != nil {
			return nil, fmt.Errorf("source file not accessible: %s: %w", req.SourcePath, err)
		}

		targetPath := filepath.Join(jailorRootPath, req.TargetName)

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
func LinkGuestFilesForTask(jailorRootPath string, req *LinkGuestFilesRequest) (map[string]string, error) {
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

	// Link all files into chroot
	return LinkFilesIntoJail(jailorRootPath, linkRequests)
}
