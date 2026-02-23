package machine

import (
	"context"
	"testing"
)

func TestNilClient(t *testing.T) {
	var c *Client
	ctx := context.Background()

	if err := c.PauseVM(ctx); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.CreateSnapshot(ctx, "/snap", "/mem"); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.LoadSnapshot(ctx, "/snap", "/mem"); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.SendCtrlAltDel(ctx); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PutMmds(ctx, nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if _, err := c.GetMachineConfiguration(); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PutMachineConfiguration(ctx, nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PutBootSource(ctx, nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PutDrive(ctx, "d", nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PutNetworkInterface(ctx, "n", nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PutMmdsConfig(ctx, nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PutBalloon(ctx, nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.PatchBalloon(ctx, nil); err == nil {
		t.Fatal("expected error from nil client")
	}
	if err := c.StartInstance(ctx); err == nil {
		t.Fatal("expected error from nil client")
	}
}
