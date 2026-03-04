// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package machine

import (
	"fmt"
	"os"

	drivers "github.com/hashicorp/nomad/plugins/drivers"
)

// AttachHostVolumes converts Nomad host volume mounts into Firecracker
// drives and guest mount metadata. Only block devices are supported.
// existingDriveCount is the number of user-configured drives already
// present; it determines the virtio-blk device letter offset.
func AttachHostVolumes(mounts []*drivers.MountConfig, existingDriveCount int) ([]Drive, []GuestMount, error) {
	var drives []Drive
	var guestMounts []GuestMount

	for _, m := range mounts {
		info, err := os.Stat(m.HostPath)
		if err != nil {
			return nil, nil, fmt.Errorf("volume mount %q: %w", m.HostPath, err)
		}
		if info.Mode()&os.ModeDevice == 0 {
			return nil, nil, fmt.Errorf("volume mount %q is not a block device; the firecracker driver only supports block device volume mounts", m.HostPath)
		}

		driveIdx := existingDriveCount + len(drives)
		if driveIdx > 25 {
			return nil, nil, fmt.Errorf("too many drives (%d); maximum 26 virtio-blk devices supported", driveIdx+1)
		}
		devLetter := string(rune('a' + driveIdx))

		drives = append(drives, Drive{
			PathOnHost:   m.HostPath,
			IsRootDevice: false,
			IsReadOnly:   m.Readonly,
		})
		guestMounts = append(guestMounts, GuestMount{
			DevicePath: "/dev/vd" + devLetter,
			MountPath:  m.TaskPath,
		})
	}

	return drives, guestMounts, nil
}
