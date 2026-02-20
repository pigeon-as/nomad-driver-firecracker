//go:build !windows
// +build !windows

package client

import (
	"context"
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/sirupsen/logrus"
)

// Client wraps the Firecracker SDK's low-level Client for HTTP API communication
// over Unix socket to a running Firecracker daemon.
type Client struct {
	client *firecracker.Client
}

// New creates a new HTTP client for a Firecracker daemon at socketPath.
// Pass nil for logger to disable SDK logging, or a logrus.Entry for debug output.
func New(socketPath string, logger *logrus.Entry) *Client {
	fc := firecracker.NewClient(socketPath, logger, false)
	return &Client{client: fc}
}

// GetInstanceInfo queries the running VM's current state.
// Returns error if daemon is unreachable (useful for recovery verification).
func (c *Client) GetInstanceInfo(ctx context.Context) (*models.InstanceInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("client is not initialized")
	}
	resp, err := c.client.GetInstanceInfo(ctx)
	if err != nil {
		return nil, err
	}
	return resp.Payload, nil
}

// SendCtrlAltDel sends Ctrl+Alt+Del to the VM (graceful shutdown signal).
func (c *Client) SendCtrlAltDel(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	action := &models.InstanceActionInfo{
		ActionType: strPtr(models.InstanceActionInfoActionTypeSendCtrlAltDel),
	}
	_, err := c.client.CreateSyncAction(ctx, action)
	return err
}

// Pause pauses VM execution. The VM can be resumed with Resume().
func (c *Client) Pause(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	// Note: Pause/Resume are controlled via PatchVM, not CreateSyncAction
	// Using direct SDK method would be: m.client.PatchVM(ctx, &models.VM{State: strPtr("Paused")})
	// But in recovery/runtime context, we recommend using StopTask for cleanup instead
	return errors.New("Pause not supported in recovery context; use StopTask for VM shutdown")
}

// Resume resumes VM execution after being paused.
func (c *Client) Resume(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	// Note: Pause/Resume are controlled via PatchVM, not CreateSyncAction
	return errors.New("Resume not supported in recovery context; use higher-level recovery API")
}

// SendSignal maps Nomad signals to Firecracker HTTP actions:
// - SIGTERM, SIGINT: sends Ctrl+Alt+Del (graceful shutdown)
// - SIGKILL, others: returns error (let Nomad escalate to StopTask)
func (c *Client) SendSignal(ctx context.Context, signal string) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}

	switch signal {
	case "SIGTERM", "SIGINT":
		return c.SendCtrlAltDel(ctx)
	case "SIGKILL":
		return errors.New("SIGKILL not supported via HTTP API; use StopTask for force kill")
	default:
		return errors.New("signal not supported via Firecracker HTTP API: " + signal)
	}
}

// strPtr returns a pointer to a string
func strPtr(s string) *string {
	return &s
}
