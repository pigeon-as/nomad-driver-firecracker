package jailer

import (
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

type JailerConfig struct {
	ExecFile     string `codec:"exec_file"`
	JailerBinary string `codec:"jailer_binary"`
}

func (c *JailerConfig) Validate() error {

	if c == nil {
		return nil
	}

	if c.ExecFile == "" {
		c.ExecFile = "firecracker"
	}
	if c.JailerBinary == "" {
		c.JailerBinary = "jailer"
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
