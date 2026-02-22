package network_interface

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestNetworkInterfaces_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		n := NetworkInterfaces{{
			StaticConfiguration: &StaticNetworkConfiguration{HostDevName: "tap0"},
		}}
		must.NoError(t, n.Validate())
	})
	t.Run("missing static config", func(t *testing.T) {
		n := NetworkInterfaces{{}}
		must.Error(t, n.Validate())
	})
	t.Run("missing host_dev_name", func(t *testing.T) {
		n := NetworkInterfaces{{
			StaticConfiguration: &StaticNetworkConfiguration{},
		}}
		must.Error(t, n.Validate())
	})
	t.Run("valid mac", func(t *testing.T) {
		n := NetworkInterfaces{{
			StaticConfiguration: &StaticNetworkConfiguration{
				HostDevName: "tap0",
				MacAddress:  "02:fc:00:00:00:01",
			},
		}}
		must.NoError(t, n.Validate())
	})
	t.Run("invalid mac", func(t *testing.T) {
		n := NetworkInterfaces{{
			StaticConfiguration: &StaticNetworkConfiguration{
				HostDevName: "tap0",
				MacAddress:  "not-a-mac",
			},
		}}
		must.Error(t, n.Validate())
	})
}
