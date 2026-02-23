package machine

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestBootSource_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		b := &BootSource{KernelImagePath: "/vmlinux"}
		must.NoError(t, b.Validate())
	})
	t.Run("nil", func(t *testing.T) {
		var b *BootSource
		must.NoError(t, b.Validate())
	})
	t.Run("missing kernel", func(t *testing.T) {
		b := &BootSource{}
		must.Error(t, b.Validate())
	})
}

func TestDrive_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		d := &Drive{PathOnHost: "/rootfs.ext4"}
		must.NoError(t, d.Validate())
	})
	t.Run("nil", func(t *testing.T) {
		var d *Drive
		must.NoError(t, d.Validate())
	})
	t.Run("missing path", func(t *testing.T) {
		d := &Drive{}
		must.Error(t, d.Validate())
	})
}
