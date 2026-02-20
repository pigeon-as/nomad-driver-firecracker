//go:build !windows
// +build !windows

package client

import (
	"context"
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/go-hclog"
)

// Client wraps the Firecracker SDK's low-level Client for HTTP API communication
// over Unix socket to a running Firecracker daemon.
type Client struct {
	client *firecracker.Client
}

// New creates a new HTTP client for a Firecracker daemon at socketPath.
// logger parameter is unused (Firecracker SDK requires logrus, we disable SDK logging).
func New(socketPath string, logger hclog.Logger) *Client {
	// Firecracker SDK expects logrus.Entry; pass nil to disable SDK logging
	fc := firecracker.NewClient(socketPath, nil, false)
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

// Pause pauses VM execution. Prepare for CreateSnapshot.
func (c *Client) Pause(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	vm := &models.VM{
		State: strPtr(models.VMStatePaused),
	}
	_, err := c.client.PatchVM(ctx, vm)
	return err
}

// Resume resumes a paused VM (after loading from snapshot or normal pause).
func (c *Client) Resume(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	vm := &models.VM{
		State: strPtr(models.VMStateResumed),
	}
	_, err := c.client.PatchVM(ctx, vm)
	return err
}

// CreateSnapshot persists VM memory and hardware state to files.
// VM must be paused before calling this.
func (c *Client) CreateSnapshot(ctx context.Context, memPath, snapPath string) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}

	params := &models.SnapshotCreateParams{
		MemFilePath:  strPtr(memPath),
		SnapshotPath: strPtr(snapPath),
	}
	_, err := c.client.CreateSnapshot(ctx, params)
	return err
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
