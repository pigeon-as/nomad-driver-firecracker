//go:build !windows
// +build !windows

package client

import (
	"context"
	"errors"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
)

// Client wraps the Firecracker SDK's Machine to provide VM lifecycle control
// via the HTTP API over Unix socket.
type Client struct {
	machine *firecracker.Machine
}

// New creates a new Firecracker client for the VM listening on socketPath.
func New(socketPath string) *Client {
	m := firecracker.NewMachine(
		firecracker.Config{
			SocketPath:      socketPath,
			Timeout:         5 * time.Second,
			UsesKernelpath:  false,
			EnableKernelBp:  false,
			VmlinuxPath:     "",
			DisableValidate: false,
		},
		firecracker.WithClient(firecracker.NewHTTPClient(socketPath)),
	)

	return &Client{
		machine: m,
	}
}

// Shutdown gracefully shuts down the Firecracker VM.
func (c *Client) Shutdown(ctx context.Context) error {
	if c == nil || c.machine == nil {
		return errors.New("client is not initialized")
	}
	return c.machine.Shutdown(ctx)
}

// Pause pauses VM execution. The VM can be resumed with Resume().
func (c *Client) Pause(ctx context.Context) error {
	if c == nil || c.machine == nil {
		return errors.New("client is not initialized")
	}
	return c.machine.Pause(ctx)
}

// Resume resumes VM execution after being paused.
func (c *Client) Resume(ctx context.Context) error {
	if c == nil || c.machine == nil {
		return errors.New("client is not initialized")
	}
	return c.machine.Resume(ctx)
}

// SendSignal sends a signal to the VM. Maps Nomad signals to Firecracker actions:
// - SIGTERM, SIGINT: sends Ctrl+Alt+Del (graceful shutdown)
// - Other signals: returns error (not supported via HTTP; use executor for force kill)
func (c *Client) SendSignal(ctx context.Context, signal string) error {
	if c == nil || c.machine == nil {
		return errors.New("client is not initialized")
	}

	// Map Nomad signal strings to Firecracker actions
	switch signal {
	case "SIGTERM", "SIGINT":
		// sendCtrlAltDel is the graceful shutdown signal for Firecracker
		return c.machine.SendCtrlAltDel(ctx)
	case "SIGKILL":
		// SIGKILL should be handled by StopTask (via executor force-kill)
		return errors.New("SIGKILL not supported via HTTP API; use StopTask for force kill")
	default:
		// Other signals not supported via HTTP
		return errors.New("signal not supported via Firecracker HTTP API: " + signal)
	}
}
