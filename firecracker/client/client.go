package client

import (
	"context"
	"errors"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

type Client struct {
	client *firecracker.Client
}

func New(socketPath string) *Client {
	fc := firecracker.NewClient(socketPath, nil, false)
	return &Client{client: fc}
}

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

func strPtr(s string) *string {
	return &s
}
