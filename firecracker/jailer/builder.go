package jailer

import (
	"errors"
	"path/filepath"

	"github.com/firecracker-microvm/firecracker-go-sdk"
)

// BuildArgs uses the firecracker-go-sdk's command builder to construct the
// argument list for invoking the jailer.  The base directory is forced inside
// the allocation directory so that the Nomad and jailer chroots coincide.  A
// pointer to BuildParams may be provided to inject task-specific overrides.
//
// Additional arguments preceded by `--` may be provided for the Firecracker
// binary itself; these are appended after the `--` separator by the SDK
// builder.  Typically this is used to supply `--config-file`.
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

	// apply overrides from BuildParams; UID/GID are only taken from params
	if params != nil {
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
