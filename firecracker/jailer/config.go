package jailer

import (
	"errors"
	"path/filepath"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// JailerConfig contains the static values supplied via the plugin
// configuration block.  UID/GID are deliberately *not* exposed here: the
// Nomad task's `user` field is considered authoritative and is mapped to
// numeric IDs at task startup.
//
// The driver will always ignore a user-specified chroot base directory and
// compute one inside the allocation workspace.
type JailerConfig struct {
	// ExecFile is the path to the Firecracker binary that will be exec-ed by
	// the jailer. The user can provide a path to any binary, but the interaction
	// with the jailer is mostly Firecracker specific.
	ExecFile string

	// JailerBinary specifies the jailer binary to be used for setting up the
	// Firecracker VM jail. If the value contains no path separators, it will
	// use the PATH environment variable to look it up; otherwise the provided
	// path is used directly.  This must always be set by the user.
	JailerBinary string

	// ChrootBaseDir represents the base folder where chroot jails are built.  It
	// is ignored by the driver, which forces the directory to be inside the
	// Nomad allocation work dir on a per-task basis.
	ChrootBaseDir string
}

func (n *JailerConfig) Validate() error {

	if n == nil {
		return nil
	}

	var mErr multierror.Error

	// provide sensible defaults if the user omitted the values; the SDK is
	// happy to look up a bare binary via PATH so we avoid forcing absolute
	// names on operators.
	if n.ExecFile == "" {
		n.ExecFile = "firecracker"
	}
	if n.JailerBinary == "" {
		n.JailerBinary = "jailer"
	}

	// we ignore any user-supplied chroot base dir; the driver will compute one
	// on a per-task basis and not rely on the configured value.
	n.ChrootBaseDir = ""

	return mErr.ErrorOrNil()
}

// HCLSpec returns the HCL schema for the jailer configuration. This can
// be embedded in a larger plugin/task schema when validating user input.
func HCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"exec_file":     hclspec.NewAttr("exec_file", "string", false),
		"jailer_binary": hclspec.NewAttr("jailer_binary", "string", false),
		// uid/gid removed – Nomad's user field is used instead
	})
}

// BuildParams contains dynamic values that can be passed to BuildArgs to
// influence the generated command.  They are typically derived from the
// Nomad task environment (user id, network namespace, etc.) and may override
// the static fields supplied in the plugin configuration.
type BuildParams struct {
	// UID/GID override values.  If nil the plugin config value (if any) will
	// be used instead.
	UID *int
	GID *int

	// NetNS is a network namespace path that the jailer should join.
	NetNS string

	// CgroupVersion optionally specifies which cgroup version to request.
	// Nomad doesn't presently surface this, but we keep the field for
	// forward‑compatibility with the SDK.
	CgroupVersion string
}

// BuildArgs uses the firecracker-go-sdk's command builder to construct the
// argument list for invoking the jailer.  The base directory is forced inside
// the allocation directory so that the Nomad and jailer chroots coincide.  A
// pointer to BuildParams may be provided to inject task-specific overrides.
func (c *JailerConfig) BuildArgs(allocDir string, params *BuildParams) ([]string, error) {
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

	return builder.Args(), nil
}

// Bin returns the configured jailer binary path.
func (c *JailerConfig) Bin() string {
	if c == nil {
		return ""
	}
	return c.JailerBinary
}
