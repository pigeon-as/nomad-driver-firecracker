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
		"network_interface": hclspec.NewBlockList("network_interface", hclspec.NewObject(map[string]*hclspec.Spec{
			"static_configuration": hclspec.NewBlock("static_configuration", true, hclspec.NewObject(map[string]*hclspec.Spec{
				"mac_address":   hclspec.NewAttr("mac_address", "string", false),
				"host_dev_name": hclspec.NewAttr("host_dev_name", "string", true),
				"ip_configuration": hclspec.NewBlock("ip_configuration", false, hclspec.NewObject(map[string]*hclspec.Spec{
					"ip_addr":     hclspec.NewAttr("ip_addr", "string", true),
					"gateway":     hclspec.NewAttr("gateway", "string", true),
					"nameservers": hclspec.NewAttr("nameservers", "list(string)", false),
					"if_name":     hclspec.NewAttr("if_name", "string", false),
				})),
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
	MacAddress      string           `codec:"mac_address"`
	HostDevName     string           `codec:"host_dev_name"`
	IPConfiguration *IPConfiguration `codec:"ip_configuration"`
}

func (staticConf StaticNetworkConfiguration) validate() error {
	if staticConf.HostDevName == "" {
		return fmt.Errorf("host_dev_name must be provided if static_configuration is provided: %+v", staticConf)
	}
	if staticConf.IPConfiguration != nil {
		if err := staticConf.IPConfiguration.validate(); err != nil {
			return err
		}
	}
	return nil
}

type IPConfiguration struct {
	IPAddr      string   `codec:"ip_addr"` // String format: "192.168.1.10/24"
	Gateway     string   `codec:"gateway"` // String format: "192.168.1.1"
	Nameservers []string `codec:"nameservers"`
	IfName      string   `codec:"if_name"`
}

func (ipConf IPConfiguration) validate() error {
	if ipConf.IPAddr == "" {
		return fmt.Errorf("ip_addr is required")
	}
	// Parse and validate CIDR notation
	ip, _, err := net.ParseCIDR(ipConf.IPAddr)
	if err != nil || ip.To4() == nil {
		return fmt.Errorf("invalid ip_addr, must be valid IPv4 CIDR notation (e.g., 192.168.1.10/24): %s", ipConf.IPAddr)
	}

	if ipConf.Gateway == "" {
		return fmt.Errorf("gateway is required")
	}
	// Parse and validate gateway IP
	gwIP := net.ParseIP(ipConf.Gateway)
	if gwIP == nil || gwIP.To4() == nil {
		return fmt.Errorf("invalid gateway, must be valid IPv4 address (e.g., 192.168.1.1): %s", ipConf.Gateway)
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
