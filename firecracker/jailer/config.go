package jailer

import (
	"fmt"

	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

type JailerConfig struct {
	ExecFile     string `codec:"exec_file"`
	JailerBinary string `codec:"jailer_binary"`
	// ChrootBase is the base directory for jailer chroot directories.
	// Must be short to keep the Firecracker API socket path under the
	// Unix domain socket sun_path limit (107 bytes). Defaults to
	// /srv/jailer, matching the firecracker-go-sdk convention.
	ChrootBase string `codec:"chroot_base"`
}

func (c *JailerConfig) Validate() error {
	if c == nil {
		return nil
	}

	// Defaults are applied via HCLSpec during config decode.
	if c.ExecFile == "" {
		return fmt.Errorf("exec_file must be specified")
	}
	if c.JailerBinary == "" {
		return fmt.Errorf("jailer_binary must be specified")
	}
	if c.ChrootBase == "" {
		return fmt.Errorf("chroot_base must be specified")
	}

	return nil
}

func HCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"exec_file": hclspec.NewDefault(
			hclspec.NewAttr("exec_file", "string", false),
			hclspec.NewLiteral(`"firecracker"`),
		),
		"jailer_binary": hclspec.NewDefault(
			hclspec.NewAttr("jailer_binary", "string", false),
			hclspec.NewLiteral(`"jailer"`),
		),
		"chroot_base": hclspec.NewDefault(
			hclspec.NewAttr("chroot_base", "string", false),
			hclspec.NewLiteral(`"/srv/jailer"`),
		),
	})
}

type BuildParams struct {
	ID            string
	UID           *int
	GID           *int
	NetNS         string
	CgroupVersion string
}

func (c *JailerConfig) Bin() string {
	if c == nil {
		return ""
	}
	return c.JailerBinary
}
