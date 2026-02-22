package client

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func WaitForReady(ctx context.Context, socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	c := New(socketPath)

	for {
		select {
		case <-ticker.C:
			if _, err := c.GetMachineConfiguration(); err == nil {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("firecracker socket not ready after %v", timeout)
			}
		case <-ctx.Done():
			return fmt.Errorf("socket verification cancelled")
		}
	}
}

func strPtr(s string) *string {
	return &s
}
