package network

import (
	"fmt"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
)

func HCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"network_interface": hclspec.NewBlockList("network_interface", hclspec.NewObject(map[string]*hclspec.Spec{
			"static_configuration": hclspec.NewBlock("static_configuration", true, hclspec.NewObject(map[string]*hclspec.Spec{
				"mac_address":   hclspec.NewAttr("mac_address", "string", false),
				"host_dev_name": hclspec.NewAttr("host_dev_name", "string", true),
			})),
		})),
	})
}

type NetworkInterfaces []NetworkInterface

type NetworkInterface struct {
	StaticConfiguration *StaticNetworkConfiguration `codec:"static_configuration"`
	InRateLimiter       *models.RateLimiter         `codec:"in_rate_limiter"`
	OutRateLimiter      *models.RateLimiter         `codec:"out_rate_limiter"`
}

type StaticNetworkConfiguration struct {
	MacAddress  string `codec:"mac_address"`
	HostDevName string `codec:"host_dev_name"`
}

func (staticConf StaticNetworkConfiguration) validate() error {
	if staticConf.HostDevName == "" {
		return fmt.Errorf("host_dev_name must be provided if static_configuration is provided: %+v", staticConf)
	}
	return nil
}

func (networkInterfaces NetworkInterfaces) Validate() error {
	return networkInterfaces.validate()
}

func (networkInterfaces NetworkInterfaces) validate() error {
	for _, iface := range networkInterfaces {
		if iface.StaticConfiguration == nil {
			return fmt.Errorf("static_configuration is required for each network interface: %+v", iface)
		}
		if err := iface.StaticConfiguration.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (networkInterfaces NetworkInterfaces) ToSDK() []*models.NetworkInterface {
	if len(networkInterfaces) == 0 {
		return nil
	}
	out := make([]*models.NetworkInterface, len(networkInterfaces))
	for i, iface := range networkInterfaces {
		m := &models.NetworkInterface{}
		if iface.StaticConfiguration != nil {
			if iface.StaticConfiguration.HostDevName != "" {
				m.HostDevName = utils.String(iface.StaticConfiguration.HostDevName)
			}
			if iface.StaticConfiguration.MacAddress != "" {
				m.GuestMac = iface.StaticConfiguration.MacAddress
			}
		}
		m.IfaceID = utils.String(fmt.Sprintf("eth%d", i))
		if iface.InRateLimiter != nil {
			m.RxRateLimiter = iface.InRateLimiter
		}
		if iface.OutRateLimiter != nil {
			m.TxRateLimiter = iface.OutRateLimiter
		}
		out[i] = m
	}
	return out
}
