// Package network contains configuration helpers for any network-related
// settings the Firecracker driver may require. Historically the driver did not
// expose any task-level network configuration, hence this package was originally
// created as an empty placeholder so that future expansion would not force a
// breaking import path change.
//
// Starting with the current release we allow users to provide a
// `network_interface` stanza in the task configuration. The driver accepts
// static tap device configuration and relies on Nomad's network isolation
// to manage CNI and netns creation.

package network

import (
	"fmt"
	"net"

	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
)

// Config holds global network-related configuration for the plugin.  It is
// currently unused but kept as a placeholder in case we need to surface
// cluster-wide settings later.
type Config struct {
}

// Validate normalizes and validates a network config.  Today it's a no-op but
// we include it for symmetry with jailer.Config and to simplify future
// extension.
func (c *Config) Validate() error {
	return nil
}

// HCLSpec returns the HCL schema for the network configuration.  It is empty
// today and provided for future compatibility.
func HCLSpec() *hclspec.Spec {
	return hclspec.NewObject(map[string]*hclspec.Spec{
		// TODO: define attributes once they exist
	})
}

//-----------
// public API
//-----------

// NetworkInterfaces mirrors the Firecracker SDK type and is exposed in the
// task configuration.  Clients should not import the SDK directly; this type
// lets us evolve independently and add Nomad-specific helpers later.
type NetworkInterfaces []NetworkInterface

// NetworkInterface represents an interface within the microVM.
type NetworkInterface struct {
	StaticConfiguration *StaticNetworkConfiguration `codec:"static_configuration"`
	AllowMMDS           bool                        `codec:"allow_mmds"`
	InRateLimiter       *models.RateLimiter         `codec:"in_rate_limiter"`
	OutRateLimiter      *models.RateLimiter         `codec:"out_rate_limiter"`
}

// StaticNetworkConfiguration allows a network interface to be defined via
// static parameters.
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

// IPConfiguration specifies an IP, gateway and DNS nameservers that should be
// configured automatically within the VM upon boot.  Only IPv4 is supported.
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

// Validate is a convenience wrapper around the unexported helpers.
func (networkInterfaces NetworkInterfaces) Validate() error {
	return networkInterfaces.validate()
}

// validate performs semantic checking of the configuration.
// Nomad is responsible for network namespaces and CNI execution; the driver
// only accepts static tap device configuration.
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

// ToSDK converts a slice of NetworkInterface values into the SDK's
// representation.  This mirrors the old convertNetwork helper that lived in
// driver.go but keeps the logic close to the type definition.
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
