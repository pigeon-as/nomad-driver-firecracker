package machine

import (
	"encoding/json"
	"testing"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
	"github.com/shoenig/test/must"
)

func TestBuildMmdsContent(t *testing.T) {
	guestNets := []network.GuestNetworkConfig{
		{IP: "172.26.64.2", Mask: 20, Gateway: "172.26.64.1"},
	}

	dualStack := []network.GuestNetworkConfig{
		{IP: "172.26.64.2", Mask: 20, Gateway: "172.26.64.1"},
		{IP: "fdaa:0:a1b2:c3d4::5", Mask: 64, Gateway: "fdaa:0:a1b2:c3d4::1"},
	}

	t.Run("nil when no metadata and no network and no mounts", func(t *testing.T) {
		result := BuildMmdsContent("", nil, nil)
		must.Nil(t, result)
	})

	t.Run("network only", func(t *testing.T) {
		result := BuildMmdsContent("", guestNets, nil)
		m := result.(map[string]interface{})
		must.MapContainsKey(t, m, "IPConfigs")

		b, _ := json.Marshal(result)
		must.StrContains(t, string(b), `"IP":"172.26.64.2"`)
		must.StrContains(t, string(b), `"Gateway":"172.26.64.1"`)
		must.StrContains(t, string(b), `"Mask":20`)
	})

	t.Run("user metadata only", func(t *testing.T) {
		result := BuildMmdsContent(`{"app":"test"}`, nil, nil)
		m := result.(map[string]interface{})
		must.Eq(t, "test", m["app"])
		must.MapNotContainsKey(t, m, "IPConfigs")
	})

	t.Run("user metadata merged with network", func(t *testing.T) {
		result := BuildMmdsContent(`{"app":"test"}`, guestNets, nil)
		m := result.(map[string]interface{})
		must.Eq(t, "test", m["app"])
		must.MapContainsKey(t, m, "IPConfigs")
	})

	t.Run("non-array user IPConfigs replaced by network", func(t *testing.T) {
		result := BuildMmdsContent(`{"IPConfigs":"bad"}`, guestNets, nil)
		m := result.(map[string]interface{})
		configs, ok := m["IPConfigs"].([]interface{})
		must.True(t, ok)
		must.Len(t, 1, configs)
	})

	t.Run("network appends to user IPConfigs", func(t *testing.T) {
		userMeta := `{"IPConfigs":[{"Gateway":"10.0.0.1","IP":"10.0.0.2","Mask":24}]}`
		result := BuildMmdsContent(userMeta, guestNets, nil)
		m := result.(map[string]interface{})
		configs, ok := m["IPConfigs"].([]interface{})
		must.True(t, ok)
		must.Len(t, 2, configs)
	})

	t.Run("empty guestNets skips injection", func(t *testing.T) {
		result := BuildMmdsContent(`{"app":"test"}`, nil, nil)
		m := result.(map[string]interface{})
		must.MapNotContainsKey(t, m, "IPConfigs")
	})

	t.Run("dual-stack produces two IPConfigs", func(t *testing.T) {
		result := BuildMmdsContent("", dualStack, nil)
		m := result.(map[string]interface{})
		configs, ok := m["IPConfigs"].([]interface{})
		must.True(t, ok)
		must.Len(t, 2, configs)

		b, _ := json.Marshal(result)
		must.StrContains(t, string(b), `"IP":"172.26.64.2"`)
		must.StrContains(t, string(b), `"IP":"fdaa:0:a1b2:c3d4::5"`)
	})

	t.Run("dual-stack appends to user IPConfigs", func(t *testing.T) {
		userMeta := `{"IPConfigs":[{"IP":"10.0.0.99","Mask":24}]}`
		result := BuildMmdsContent(userMeta, dualStack, nil)
		m := result.(map[string]interface{})
		configs, ok := m["IPConfigs"].([]interface{})
		must.True(t, ok)
		must.Len(t, 3, configs) // 1 user + 2 dual-stack
	})

	t.Run("mounts only", func(t *testing.T) {
		mounts := []GuestMount{
			{DevicePath: "/dev/vdb", MountPath: "/data"},
		}
		result := BuildMmdsContent("", nil, mounts)
		m := result.(map[string]interface{})
		must.MapContainsKey(t, m, "Mounts")
		must.MapNotContainsKey(t, m, "IPConfigs")

		b, _ := json.Marshal(result)
		must.StrContains(t, string(b), `"DevicePath":"/dev/vdb"`)
		must.StrContains(t, string(b), `"MountPath":"/data"`)
	})

	t.Run("multiple mounts with network", func(t *testing.T) {
		mounts := []GuestMount{
			{DevicePath: "/dev/vdb", MountPath: "/data"},
			{DevicePath: "/dev/vdc", MountPath: "/cache"},
		}
		result := BuildMmdsContent("", guestNets, mounts)
		m := result.(map[string]interface{})
		must.MapContainsKey(t, m, "IPConfigs")
		must.MapContainsKey(t, m, "Mounts")

		mountList, ok := m["Mounts"].([]interface{})
		must.True(t, ok)
		must.Len(t, 2, mountList)
	})

	t.Run("all combined", func(t *testing.T) {
		mounts := []GuestMount{
			{DevicePath: "/dev/vdb", MountPath: "/data"},
		}
		result := BuildMmdsContent(`{"app":"test"}`, dualStack, mounts)
		m := result.(map[string]interface{})
		must.Eq(t, "test", m["app"])
		must.MapContainsKey(t, m, "IPConfigs")
		must.MapContainsKey(t, m, "Mounts")

		configs, ok := m["IPConfigs"].([]interface{})
		must.True(t, ok)
		must.Len(t, 2, configs) // IPv4 + IPv6 from dual-stack
	})

	t.Run("driver appends to user Mounts", func(t *testing.T) {
		userMeta := `{"Mounts":[{"DevicePath":"/dev/vdc","MountPath":"/extra"}]}`
		mounts := []GuestMount{
			{DevicePath: "/dev/vdb", MountPath: "/data"},
		}
		result := BuildMmdsContent(userMeta, nil, mounts)
		m := result.(map[string]interface{})
		mountList, ok := m["Mounts"].([]interface{})
		must.True(t, ok)
		must.Len(t, 2, mountList)
	})

	t.Run("gateway omitted when empty", func(t *testing.T) {
		nets := []network.GuestNetworkConfig{
			{IP: "fdaa:0:1234:5678::5", Mask: 64},
		}
		result := BuildMmdsContent("", nets, nil)
		m := result.(map[string]interface{})
		configs := m["IPConfigs"].([]interface{})
		entry := configs[0].(map[string]interface{})
		_, hasGW := entry["Gateway"]
		must.False(t, hasGW)
	})
}
