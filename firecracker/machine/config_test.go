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

func TestVsock_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var v *Vsock
		must.NoError(t, v.Validate())
	})
	t.Run("valid CID 3", func(t *testing.T) {
		v := &Vsock{GuestCID: 3}
		must.NoError(t, v.Validate())
	})
	t.Run("valid max CID", func(t *testing.T) {
		v := &Vsock{GuestCID: 0xFFFFFFFF}
		must.NoError(t, v.Validate())
	})
	t.Run("CID too low", func(t *testing.T) {
		v := &Vsock{GuestCID: 2}
		must.Error(t, v.Validate())
	})
	t.Run("CID too high", func(t *testing.T) {
		v := &Vsock{GuestCID: 0x100000000}
		must.Error(t, v.Validate())
	})
}

func TestMmds_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var m *Mmds
		must.NoError(t, m.Validate())
	})
	t.Run("empty", func(t *testing.T) {
		m := &Mmds{}
		must.NoError(t, m.Validate())
	})
	t.Run("valid V1", func(t *testing.T) {
		m := &Mmds{Version: "V1"}
		must.NoError(t, m.Validate())
	})
	t.Run("valid V2", func(t *testing.T) {
		m := &Mmds{Version: "V2"}
		must.NoError(t, m.Validate())
	})
	t.Run("invalid version", func(t *testing.T) {
		m := &Mmds{Version: "V3"}
		must.Error(t, m.Validate())
	})
	t.Run("valid metadata", func(t *testing.T) {
		m := &Mmds{Metadata: `{"key":"value"}`}
		must.NoError(t, m.Validate())
	})
	t.Run("invalid JSON", func(t *testing.T) {
		m := &Mmds{Metadata: "not json"}
		must.Error(t, m.Validate())
	})
	t.Run("IPConfigs allowed in user metadata", func(t *testing.T) {
		m := &Mmds{Metadata: `{"IPConfigs":[{"IP":"fdaa::1","Mask":128,"Gateway":"fdaa::gw"}]}`}
		must.NoError(t, m.Validate())
	})
	t.Run("Mounts allowed in user metadata", func(t *testing.T) {
		m := &Mmds{Metadata: `{"Mounts":[{"DevicePath":"/dev/vdc","MountPath":"/extra"}]}`}
		must.NoError(t, m.Validate())
	})
}
