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

// Client wraps the firecracker-go-sdk HTTP client for the Firecracker
// API socket.
type Client struct {
	client *firecracker.Client
}

// New creates a Firecracker API client for the given socket path.
func New(socketPath string) *Client {
	return &Client{
		client: firecracker.NewClient(socketPath, nil, false),
	}
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

// PauseVM pauses the microVM by setting its state to "Paused".
func (c *Client) PauseVM(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	vm := &models.VM{State: strPtr(models.VMStatePaused)}
	_, err := c.client.PatchVM(ctx, vm)
	return err
}

// CreateSnapshot creates a full snapshot of the paused microVM.
// Paths are relative to the Firecracker chroot root.
func (c *Client) CreateSnapshot(ctx context.Context, snapshotPath, memFilePath string) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	params := &models.SnapshotCreateParams{
		SnapshotPath: &snapshotPath,
		MemFilePath:  &memFilePath,
	}
	_, err := c.client.CreateSnapshot(ctx, params)
	return err
}

// LoadSnapshot loads a previously saved snapshot and resumes the VM.
// Paths are relative to the Firecracker chroot root.
func (c *Client) LoadSnapshot(ctx context.Context, snapshotPath, memFilePath string) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	params := &models.SnapshotLoadParams{
		SnapshotPath: &snapshotPath,
		MemFilePath:  &memFilePath,
		ResumeVM:     true,
	}
	_, err := c.client.LoadSnapshot(ctx, params)
	return err
}

// PutMachineConfiguration sets the vCPU count and memory via the
// Firecracker API. Must be called before InstanceStart.
func (c *Client) PutMachineConfiguration(ctx context.Context, cfg *models.MachineConfiguration) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	_, err := c.client.PutMachineConfiguration(ctx, cfg)
	return err
}

// PutBootSource sets the kernel, initrd, and boot args via the
// Firecracker API. Must be called before InstanceStart.
func (c *Client) PutBootSource(ctx context.Context, src *models.BootSource) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	_, err := c.client.PutGuestBootSource(ctx, src)
	return err
}

// PutDrive attaches a drive to the VM via the Firecracker API.
// Must be called before InstanceStart.
func (c *Client) PutDrive(ctx context.Context, driveID string, drive *models.Drive) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	_, err := c.client.PutGuestDriveByID(ctx, driveID, drive)
	return err
}

// PutNetworkInterface attaches a network interface via the Firecracker API.
// Must be called before InstanceStart.
func (c *Client) PutNetworkInterface(ctx context.Context, ifaceID string, iface *models.NetworkInterface) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	_, err := c.client.PutGuestNetworkInterfaceByID(ctx, ifaceID, iface)
	return err
}

// PutMmdsConfig configures the MMDS network interface routing.
// Must be called before InstanceStart.
func (c *Client) PutMmdsConfig(ctx context.Context, cfg *models.MmdsConfig) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	_, err := c.client.PutMmdsConfig(ctx, cfg)
	return err
}

// StartInstance boots the microVM by sending an InstanceStart action.
// All pre-boot configuration (machine config, boot source, drives,
// network interfaces) must be applied before calling this.
func (c *Client) StartInstance(ctx context.Context) error {
	if c == nil || c.client == nil {
		return errors.New("client is not initialized")
	}
	action := &models.InstanceActionInfo{
		ActionType: strPtr(models.InstanceActionInfoActionTypeInstanceStart),
	}
	_, err := c.client.CreateSyncAction(ctx, action)
	return err
}

// ConfigureVM applies a full VM configuration via sequential API calls,
// following the firecracker-go-sdk handler chain order:
// PutMachineConfiguration → PutBootSource → PutDrive (each) →
// PutNetworkInterface (each) → PutMmdsConfig → StartInstance.
func (c *Client) ConfigureVM(ctx context.Context, cfg *models.FullVMConfiguration) error {
	if cfg.MachineConfig != nil {
		if err := c.PutMachineConfiguration(ctx, cfg.MachineConfig); err != nil {
			return fmt.Errorf("PUT /machine-config: %w", err)
		}
	}
	if cfg.BootSource != nil {
		if err := c.PutBootSource(ctx, cfg.BootSource); err != nil {
			return fmt.Errorf("PUT /boot-source: %w", err)
		}
	}
	for _, d := range cfg.Drives {
		id := ""
		if d.DriveID != nil {
			id = *d.DriveID
		}
		if err := c.PutDrive(ctx, id, d); err != nil {
			return fmt.Errorf("PUT /drives/%s: %w", id, err)
		}
	}
	for _, n := range cfg.NetworkInterfaces {
		id := ""
		if n.IfaceID != nil {
			id = *n.IfaceID
		}
		if err := c.PutNetworkInterface(ctx, id, n); err != nil {
			return fmt.Errorf("PUT /network-interfaces/%s: %w", id, err)
		}
	}
	if cfg.MmdsConfig != nil {
		if err := c.PutMmdsConfig(ctx, cfg.MmdsConfig); err != nil {
			return fmt.Errorf("PUT /mmds/config: %w", err)
		}
	}
	if err := c.StartInstance(ctx); err != nil {
		return fmt.Errorf("PUT /actions (InstanceStart): %w", err)
	}
	return nil
}

// WaitForReady polls until the Firecracker API socket is ready. It mirrors
// the firecracker-go-sdk's waitForSocket: first os.Stat to check the file
// exists, then GetMachineConfiguration to verify the API is responding.
// The SDK defaults to a 3s timeout with 10ms polling.
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
				return fmt.Errorf("socket verification cancelled: %w", ctx.Err())
			}
			return fmt.Errorf("firecracker socket not ready after %v", timeout)
		case <-ticker.C:
			// Phase 1: check socket file exists (cheap syscall).
			if _, err := os.Stat(socketPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				// Permission errors, invalid paths, etc. are hard failures.
				return fmt.Errorf("failed to stat firecracker socket %q: %w", socketPath, err)
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
