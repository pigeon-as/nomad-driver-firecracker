package machine

import (
	"context"
	"testing"

	"github.com/shoenig/test/must"
)

func TestNilClient(t *testing.T) {
	var c *Client
	ctx := context.Background()

	must.Error(t, c.PauseVM(ctx))
	must.Error(t, c.CreateSnapshot(ctx, "/snap", "/mem"))
	must.Error(t, c.LoadSnapshot(ctx, "/snap", "/mem"))
	must.Error(t, c.SendCtrlAltDel(ctx))
	must.Error(t, c.PutMmds(ctx, nil))
	_, err := c.GetMachineConfiguration()
	must.Error(t, err)
	must.Error(t, c.PutMachineConfiguration(ctx, nil))
	must.Error(t, c.PutBootSource(ctx, nil))
	must.Error(t, c.PutDrive(ctx, "d", nil))
	must.Error(t, c.PutNetworkInterface(ctx, "n", nil))
	must.Error(t, c.PutMmdsConfig(ctx, nil))
	must.Error(t, c.PutLogger(ctx, nil))
	must.Error(t, c.PutBalloon(ctx, nil))
	must.Error(t, c.PutVsock(ctx, nil))
	must.Error(t, c.StartInstance(ctx))
}
