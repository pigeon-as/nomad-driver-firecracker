package guestapi

import (
	"errors"
	"path/filepath"

	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/machine"
)

// GuestAPI configures the vsock guest agent integration. Presence of
// this block opts in to vsock-based signal delivery and status queries.
// Requires a vsock block to also be configured.
type GuestAPI struct {
	Port uint32 `codec:"port"`
}

// Validate checks the guest_api configuration.
func (g *GuestAPI) Validate() error {
	if g == nil {
		return nil
	}
	if g.Port == 0 {
		return errors.New("guest_api.port must be > 0")
	}
	return nil
}

// HCLSpec returns the HCL spec for the guest_api block.
func HCLSpec() *hclspec.Spec {
	return hclspec.NewBlock("guest_api", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"port": hclspec.NewDefault(
			hclspec.NewAttr("port", "number", false),
			hclspec.NewLiteral("10000"),
		),
	}))
}

// UDSPath derives the vsock UDS path from a Firecracker API socket path.
// The Firecracker API socket lives at <jailerDir>/root/run/firecracker.socket;
// the vsock UDS lives at <jailerDir>/root/v.sock. Returns empty string if
// socketPath is empty.
func UDSPath(socketPath string) string {
	if socketPath == "" {
		return ""
	}
	// socketPath = <jailerDir>/root/run/firecracker.socket
	// chrootRoot = <jailerDir>/root
	chrootRoot := filepath.Dir(filepath.Dir(socketPath))
	return filepath.Join(chrootRoot, machine.VsockPath)
}
