package machine

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
)

// LogFile is the filename used for Firecracker daemon logs inside the
// jailer chroot. Firecracker writes structured JSON logs here when the
// logger is configured via PUT /logger.
const LogFile = "firecracker.log"

// VsockPath is the filename used for the vsock Unix domain socket inside
// the jailer chroot. Firecracker creates and listens on this socket when
// a vsock device is configured via PUT /vsock.
const VsockPath = "v.sock"

// DefaultLogLevel is the Firecracker log verbosity used when no
// log_level is specified in the task config. Matches the Firecracker
// default.
const DefaultLogLevel = "Warning"

// BootSource describes the kernel and optional initrd for the VM.
type BootSource struct {
	KernelImagePath string `codec:"kernel_image_path"`
	BootArgs        string `codec:"boot_args"`
	InitrdPath      string `codec:"initrd_path"`
}

func (b *BootSource) Validate() error {
	if b == nil {
		return nil
	}
	if b.KernelImagePath == "" {
		return errors.New("boot_source.kernel_image_path must be provided")
	}
	return nil
}

func BootSourceHCLSpec() *hclspec.Spec {
	return hclspec.NewBlock("boot_source", true, hclspec.NewObject(map[string]*hclspec.Spec{
		"kernel_image_path": hclspec.NewAttr("kernel_image_path", "string", true),
		"boot_args":         hclspec.NewAttr("boot_args", "string", false),
		"initrd_path":       hclspec.NewAttr("initrd_path", "string", false),
	}))
}

// Drive describes a block device attached to the VM.
// Name is optional; when set it becomes the Firecracker drive_id.
// If any drive has a name, all drives must have one.
type Drive struct {
	Name         string              `codec:"name"`
	PathOnHost   string              `codec:"path_on_host"`
	IsRootDevice bool                `codec:"is_root_device"`
	IsReadOnly   bool                `codec:"is_read_only"`
	RateLimiter  *models.RateLimiter `codec:"rate_limiter"`
}

func (d *Drive) Validate() error {
	if d == nil {
		return nil
	}
	if d.PathOnHost == "" {
		return errors.New("drive.path_on_host must be set")
	}
	return nil
}

func DriveHCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"name":           hclspec.NewAttr("name", "string", false),
		"path_on_host":   hclspec.NewAttr("path_on_host", "string", true),
		"is_root_device": hclspec.NewAttr("is_root_device", "bool", false),
		"is_read_only":   hclspec.NewAttr("is_read_only", "bool", false),
		"rate_limiter":   hclspec.NewBlock("rate_limiter", false, network.RateLimiterHCLSpec()),
	})
}

// Balloon describes the virtio-balloon device for memory reclaim.
// AmountMiB is the target balloon size; the guest reclaims this much
// memory for the host. DeflateOnOOM allows the guest to reclaim
// balloon pages when under memory pressure.
type Balloon struct {
	AmountMiB            int64 `codec:"amount_mib"`
	DeflateOnOOM         bool  `codec:"deflate_on_oom"`
	StatsPollingInterval int64 `codec:"stats_polling_interval_s"`
}

func (b *Balloon) Validate() error {
	if b == nil {
		return nil
	}
	if b.AmountMiB < 0 {
		return errors.New("balloon.amount_mib must be non-negative")
	}
	if b.StatsPollingInterval < 0 {
		return errors.New("balloon.stats_polling_interval_s must be non-negative")
	}
	return nil
}

func BalloonHCLSpec() *hclspec.Spec {
	return hclspec.NewBlock("balloon", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"amount_mib":               hclspec.NewAttr("amount_mib", "number", true),
		"deflate_on_oom":           hclspec.NewAttr("deflate_on_oom", "bool", false),
		"stats_polling_interval_s": hclspec.NewAttr("stats_polling_interval_s", "number", false),
	}))
}

// Vsock enables the virtio-vsock device for host↔guest communication.
// GuestCID is the 32-bit Context Identifier for the guest; it must be
// unique per host and ≥ 3 (CID 0/1 are reserved, CID 2 is the host).
type Vsock struct {
	GuestCID int64 `codec:"guest_cid"`
}

func (v *Vsock) Validate() error {
	if v == nil {
		return nil
	}
	if v.GuestCID < 3 {
		return errors.New("vsock.guest_cid must be >= 3 (0, 1, and 2 are reserved)")
	}
	if v.GuestCID > 0xFFFFFFFF {
		return errors.New("vsock.guest_cid must fit in 32 bits (max 4294967295)")
	}
	return nil
}

func VsockHCLSpec() *hclspec.Spec {
	return hclspec.NewBlock("vsock", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"guest_cid": hclspec.NewAttr("guest_cid", "number", true),
	}))
}

// Mmds configures the Microvm Metadata Service (MMDS).
// Metadata is the raw JSON content served to the guest. Version selects
// V1 or V2 (defaults to V2). Interface overrides which network
// interface carries MMDS traffic (defaults to the first interface).
type Mmds struct {
	Version   string `codec:"version"`
	Interface string `codec:"interface"`
	Metadata  string `codec:"metadata"`
}

// GetMetadata returns the user-provided metadata string, or empty if nil.
func (m *Mmds) GetMetadata() string {
	if m == nil {
		return ""
	}
	return m.Metadata
}

func (m *Mmds) Validate() error {
	if m == nil {
		return nil
	}
	if m.Version != "" {
		switch m.Version {
		case "V1", "V2":
			// valid
		default:
			return fmt.Errorf("mmds.version must be \"V1\" or \"V2\"; got %q", m.Version)
		}
	}
	if m.Metadata != "" {
		if !json.Valid([]byte(m.Metadata)) {
			return errors.New("mmds.metadata must be valid JSON")
		}
	}
	return nil
}

func MmdsHCLSpec() *hclspec.Spec {
	return hclspec.NewBlock("mmds", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"version":   hclspec.NewDefault(hclspec.NewAttr("version", "string", false), hclspec.NewLiteral(`"V2"`)),
		"interface": hclspec.NewAttr("interface", "string", false),
		"metadata":  hclspec.NewAttr("metadata", "string", false),
	}))
}

// Config aggregates VM component configs for serialization via ToSDK.
type Config struct {
	BootSource        *BootSource
	Drives            []Drive
	NetworkInterfaces network.NetworkInterfaces
	Balloon           *Balloon
	MmdsConfig        *models.MmdsConfig
	// LogLevel sets the Firecracker daemon log verbosity. Valid values
	// are "Error", "Warning", "Info", "Debug" (case-sensitive).
	// Defaults to DefaultLogLevel ("Warning") when empty.
	LogLevel string
	// Mmds holds the MMDS block configuration: version, interface,
	// and raw JSON content.
	Mmds *Mmds
	// Vsock enables the virtio-vsock device for host↔guest communication.
	Vsock *Vsock
}
