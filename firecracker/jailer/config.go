package jailer

import (
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

type JailerConfig struct {
	ExecFile      string `codec:"exec_file"`
	JailerBinary  string `codec:"jailer_binary"`
	ChrootBaseDir string `codec:"chroot_base_dir"`
}

func (n *JailerConfig) Validate() error {

	if n == nil {
		return nil
	}

	var mErr multierror.Error

	if n.ExecFile == "" {
		n.ExecFile = "firecracker"
	}
	if n.JailerBinary == "" {
		n.JailerBinary = "jailer"
	}

	return mErr.ErrorOrNil()
}

func HCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"exec_file":     hclspec.NewAttr("exec_file", "string", false),
		"jailer_binary": hclspec.NewAttr("jailer_binary", "string", false),
	})
}

type BuildParams struct {
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
