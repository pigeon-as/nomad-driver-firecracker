package network

import (
	"fmt"
	"regexp"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// RateLimiterHCLSpec returns the HCL block spec for a token-bucket rate
// limiter. It is used by both network interface and drive configurations.
func RateLimiterHCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"bandwidth": hclspec.NewBlock("bandwidth", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"refill_time":    hclspec.NewAttr("refill_time", "number", true),
			"size":           hclspec.NewAttr("size", "number", true),
			"one_time_burst": hclspec.NewAttr("one_time_burst", "number", false),
		})),
		"ops": hclspec.NewBlock("ops", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"refill_time":    hclspec.NewAttr("refill_time", "number", true),
			"size":           hclspec.NewAttr("size", "number", true),
			"one_time_burst": hclspec.NewAttr("one_time_burst", "number", false),
		})),
	})
}

var (
	// macAddressRegex validates MAC address format (e.g., 02:fc:00:00:00:01)
	macAddressRegex = regexp.MustCompile(`^([0-9a-fA-F]{2}[:-]){5}([0-9a-fA-F]{2})$`)
)

type NetworkInterfaces []NetworkInterface

// NetworkInterface describes a VM network device.
// Name is optional; when set it becomes the Firecracker iface_id.
// If any interface has a name, all interfaces must have one.
type NetworkInterface struct {
	Name                string                      `codec:"name"`
	StaticConfiguration *StaticNetworkConfiguration `codec:"static_configuration"`
	InRateLimiter       *models.RateLimiter         `codec:"in_rate_limiter"`
	OutRateLimiter      *models.RateLimiter         `codec:"out_rate_limiter"`
}

type StaticNetworkConfiguration struct {
	MacAddress  string `codec:"mac_address"`
	HostDevName string `codec:"host_dev_name"`
}

func HCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		"name": hclspec.NewAttr("name", "string", false),
		"static_configuration": hclspec.NewBlock("static_configuration", true, hclspec.NewObject(map[string]*hclspec.Spec{
			"host_dev_name": hclspec.NewAttr("host_dev_name", "string", true),
			"mac_address":   hclspec.NewAttr("mac_address", "string", false),
		})),
		"in_rate_limiter":  hclspec.NewBlock("in_rate_limiter", false, RateLimiterHCLSpec()),
		"out_rate_limiter": hclspec.NewBlock("out_rate_limiter", false, RateLimiterHCLSpec()),
	})
}

func (staticConf StaticNetworkConfiguration) validate() error {
	if staticConf.HostDevName == "" {
		return fmt.Errorf("host_dev_name must be provided if static_configuration is provided: %+v", staticConf)
	}
	if staticConf.MacAddress != "" {
		// Validate MAC address format
		if !macAddressRegex.MatchString(staticConf.MacAddress) {
			return fmt.Errorf("invalid MAC address format (%s): expected format XX:XX:XX:XX:XX:XX or XX-XX-XX-XX-XX-XX", staticConf.MacAddress)
		}
	}
	return nil
}

func (networkInterfaces NetworkInterfaces) Validate() error {
	names := make([]string, len(networkInterfaces))
	for i, iface := range networkInterfaces {
		names[i] = iface.Name
	}
	if err := ValidateNames(names, "network interface"); err != nil {
		return err
	}

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
				m.HostDevName = strPtr(iface.StaticConfiguration.HostDevName)
			}
			if iface.StaticConfiguration.MacAddress != "" {
				m.GuestMac = iface.StaticConfiguration.MacAddress
			}
		}
		if iface.Name != "" {
			m.IfaceID = strPtr(iface.Name)
		} else {
			m.IfaceID = strPtr(fmt.Sprintf("eth%d", i))
		}
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

func strPtr(s string) *string { return &s }
