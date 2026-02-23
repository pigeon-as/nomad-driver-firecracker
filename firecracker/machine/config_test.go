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

func TestBalloon_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		b := &Balloon{AmountMiB: 128, DeflateOnOOM: true}
		must.NoError(t, b.Validate())
	})
	t.Run("nil", func(t *testing.T) {
		var b *Balloon
		must.NoError(t, b.Validate())
	})
	t.Run("negative amount", func(t *testing.T) {
		b := &Balloon{AmountMiB: -1}
		must.Error(t, b.Validate())
	})
	t.Run("negative stats interval", func(t *testing.T) {
		b := &Balloon{AmountMiB: 0, StatsPollingInterval: -1}
		must.Error(t, b.Validate())
	})
	t.Run("zero amount is valid", func(t *testing.T) {
		b := &Balloon{AmountMiB: 0, DeflateOnOOM: true}
		must.NoError(t, b.Validate())
	})
}
