package network

import (
	"fmt"
	"net"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
)

type Config struct {
}

func (c *Config) Validate() error {
	return nil
}

func HCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
	})
}
type NetworkInterfaces []NetworkInterface

type NetworkInterface struct {
	StaticConfiguration *StaticNetworkConfiguration `codec:"static_configuration"`
	AllowMMDS           bool                        `codec:"allow_mmds"`
	InRateLimiter       *models.RateLimiter         `codec:"in_rate_limiter"`
	OutRateLimiter      *models.RateLimiter         `codec:"out_rate_limiter"`
}

type StaticNetworkConfiguration struct {
	MacAddress      string           `codec:"mac_address"`
	HostDevName     string           `codec:"host_dev_name"`
	IPConfiguration *IPConfiguration `codec:"ip_configuration"`
}

func (staticConf StaticNetworkConfiguration) validate() error {
	if staticConf.HostDevName == "" {
		return fmt.Errorf("HostDevName must be provided if StaticNetworkConfiguration is provided: %+v", staticConf)
	}
	if staticConf.IPConfiguration != nil {
		if err := staticConf.IPConfiguration.validate(); err != nil {
			return err
		}
	}
	return nil
}

type IPConfiguration struct {
	IPAddr      net.IPNet `codec:"ip_addr"`
	Gateway     net.IP    `codec:"gateway"`
	Nameservers []string  `codec:"nameservers"`
	IfName      string    `codec:"if_name"`
}

func (ipConf IPConfiguration) validate() error {
	for _, ip := range []net.IP{ipConf.IPAddr.IP, ipConf.Gateway} {
		if ip.To4() == nil {
			return fmt.Errorf("invalid ip, only ipv4 addresses are supported: %+v", ip)
		}
	}
	if len(ipConf.Nameservers) > 2 {
		return fmt.Errorf("cannot specify more than 2 nameservers: %+v", ipConf.Nameservers)
	}
	return nil
}

func (networkInterfaces NetworkInterfaces) Validate() error {
	return networkInterfaces.validate()
}

func (networkInterfaces NetworkInterfaces) validate() error {
	for _, iface := range networkInterfaces {
		hasStaticInterface := iface.StaticConfiguration != nil
		if !hasStaticInterface {
			return fmt.Errorf("static_configuration is required for each network interface: %+v", iface)
		}
		if hasStaticInterface {
			if err := iface.StaticConfiguration.validate(); err != nil {
				return err
			}
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
