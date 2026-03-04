package machine

import (
	"os"
	"path/filepath"
	"testing"

	drivers "github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

func TestAttachHostVolumes_NonExistent(t *testing.T) {
	mounts := []*drivers.MountConfig{{HostPath: "/no/such/device", TaskPath: "/data"}}
	_, _, err := AttachHostVolumes(mounts, 0)
	must.Error(t, err)
	must.StrContains(t, err.Error(), "/no/such/device")
}

func TestAttachHostVolumes_NotBlockDevice(t *testing.T) {
	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "regular.txt")
	must.NoError(t, os.WriteFile(regularFile, []byte("hello"), 0644))

	mounts := []*drivers.MountConfig{{HostPath: regularFile, TaskPath: "/data"}}
	_, _, err := AttachHostVolumes(mounts, 0)
	must.Error(t, err)
	must.StrContains(t, err.Error(), "not a block device")
}

func TestAttachHostVolumes_TooManyDrives(t *testing.T) {
	// Even though the mount path doesn't exist, exceeding 26 drives is checked
	// after stat — so we can't easily test this without a real block device.
	// Instead, verify the limit constant by checking the error message mentions 26.
	// This test is intentionally light since we can't fabricate block devices
	// portably in unit tests.
	t.Skip("requires block devices; covered by e2e tests")
}
