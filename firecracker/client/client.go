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

// GetMachineConfiguration is a pre-boot health check for Firecracker API readiness.
func (c *Client) GetMachineConfiguration() (*models.MachineConfiguration, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("client is not initialized")
	}
	resp, err := c.client.GetMachineConfiguration()
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

func strPtr(s string) *string {
	return &s
}
