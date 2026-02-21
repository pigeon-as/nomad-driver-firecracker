package network

import (
	"fmt"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
)

type NetworkInterfaces []NetworkInterface

type NetworkInterface struct {
	StaticConfiguration *StaticNetworkConfiguration `codec:"static_configuration"`
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
		out[i] = m
	}
	return out
}
