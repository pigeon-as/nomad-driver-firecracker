package client

import (
	"context"
	"errors"
	"fmt"
	"os"
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

func (c *Client) PutMmds(ctx context.Context, metadata interface{}) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	_, err := c.client.PutMmds(ctx, metadata)
	return err
}

// WaitForReady polls until the Firecracker API socket is ready. It mirrors
// the firecracker-go-sdk's waitForSocket: first os.Stat to check the file
// exists, then GetMachineConfiguration to verify the API is responding.
// The SDK defaults to a 3s timeout with 10ms polling; Firecracker's
// documented SLA is socket readiness in 6-60ms (typically ~12ms).
func WaitForReady(ctx context.Context, socketPath string, timeout time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	c := New(socketPath)

	for {
		select {
		case <-timeoutCtx.Done():
			if ctx.Err() != nil {
				return fmt.Errorf("socket verification cancelled")
			}
			return fmt.Errorf("firecracker socket not ready after %v", timeout)
		case <-ticker.C:
			// Phase 1: check socket file exists (cheap syscall).
			if _, err := os.Stat(socketPath); err != nil {
				continue
			}
			// Phase 2: verify API is responding.
			if _, err := c.GetMachineConfiguration(); err != nil {
				continue
			}
			return nil
		}
	}
}

func strPtr(s string) *string {
	return &s
}
