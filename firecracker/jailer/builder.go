package jailer

import (
	"errors"
	"path/filepath"

	"github.com/firecracker-microvm/firecracker-go-sdk"
)

func (c *JailerConfig) BuildArgs(allocDir string, params *BuildParams, fcArgs ...string) ([]string, error) {
	if c == nil {
		return nil, errors.New("jailer config is nil")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}

	chroot := filepath.Join(allocDir, "jailer")
	builder := firecracker.NewJailerCommandBuilder().
		WithBin(c.JailerBinary).
		WithExecFile(c.ExecFile).
		WithChrootBaseDir(chroot)

	if params != nil {
		if params.ID != "" {
			builder = builder.WithID(params.ID)
		}
		if params.UID != nil {
			builder = builder.WithUID(*params.UID)
		}
		if params.GID != nil {
			builder = builder.WithGID(*params.GID)
		}
		if params.NetNS != "" {
			builder = builder.WithNetNS(params.NetNS)
		}
		if params.CgroupVersion != "" {
			builder = builder.WithCgroupVersion(params.CgroupVersion)
		}
	}

	if len(fcArgs) > 0 {
		builder = builder.WithFirecrackerArgs(fcArgs...)
	}

	return builder.Args(), nil
}
