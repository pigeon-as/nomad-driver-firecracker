package drive

import (
	"testing"

	"github.com/shoenig/test/must"
)

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
