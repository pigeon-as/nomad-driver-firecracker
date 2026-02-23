package client

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
}
