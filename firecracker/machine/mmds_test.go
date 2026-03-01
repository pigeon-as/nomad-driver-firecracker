package machine

import (
	"encoding/json"
	"testing"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
	"github.com/shoenig/test/must"
)

func TestBuildMmdsContent(t *testing.T) {
	guestNet := &network.GuestNetworkConfig{
		IP:      "172.26.64.2",
		Mask:    20,
		Gateway: "172.26.64.1",
	}

	t.Run("nil when no metadata and no network", func(t *testing.T) {
		result := BuildMmdsContent("", nil)
		must.Nil(t, result)
	})

	t.Run("network only", func(t *testing.T) {
		result := BuildMmdsContent("", guestNet)
		m := result.(map[string]interface{})
		must.MapContainsKey(t, m, "IPConfigs")

		b, _ := json.Marshal(result)
		must.StrContains(t, string(b), `"IP":"172.26.64.2"`)
		must.StrContains(t, string(b), `"Gateway":"172.26.64.1"`)
		must.StrContains(t, string(b), `"Mask":20`)
	})

	t.Run("user metadata only", func(t *testing.T) {
		result := BuildMmdsContent(`{"app":"test"}`, nil)
		m := result.(map[string]interface{})
		must.Eq(t, "test", m["app"])
		must.MapNotContainsKey(t, m, "IPConfigs")
	})

	t.Run("user metadata merged with network", func(t *testing.T) {
		result := BuildMmdsContent(`{"app":"test"}`, guestNet)
		m := result.(map[string]interface{})
		must.Eq(t, "test", m["app"])
		must.MapContainsKey(t, m, "IPConfigs")
	})

	t.Run("network overrides user IPConfigs", func(t *testing.T) {
		// Validate() rejects this, but verify BuildMmdsContent's behavior:
		// driver-injected IPConfigs wins.
		result := BuildMmdsContent(`{"IPConfigs":"bad"}`, guestNet)
		m := result.(map[string]interface{})
		configs, ok := m["IPConfigs"].([]interface{})
		must.True(t, ok)
		must.Len(t, 1, configs)
	})

	t.Run("nil guestNet with empty IP skips injection", func(t *testing.T) {
		emptyNet := &network.GuestNetworkConfig{IP: ""}
		result := BuildMmdsContent(`{"app":"test"}`, emptyNet)
		m := result.(map[string]interface{})
		must.MapNotContainsKey(t, m, "IPConfigs")
	})
}
