// Package network contains configuration helpers for any network-related
// settings the Firecracker driver may require. Historically the driver did not
// expose any task-level network configuration, hence this package was originally
// created as an empty placeholder so that future expansion would not force a
// breaking import path change.
//
// Starting with the current release we allow users to provide a `network`
// stanza in the task configuration that mirrors the Firecracker API (see
// `firecracker-go-sdk`'s `NetworkInterfaces` type).  The driver decodes and
// validates that configuration directly; no bespoke schema is required.  The
// network package remains available for additional helper logic (for example,
// converting to command-line flags or JSON documents) should we need it
// later.

package network

import (
	"fmt"
	"net"

	"github.com/containernetworking/cni/libcni"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
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

// CNIConfiguration specifies the CNI parameters that will be used to generate
// the network namespace and tap device used by a Firecracker interface.
type CNIConfiguration struct {
	NetworkName   string                    `codec:"network_name"`
	NetworkConfig *libcni.NetworkConfigList `codec:"network_config"`
	IfName        string                    `codec:"if_name"`
	VMIfName      string                    `codec:"vm_if_name"`
	Args          [][2]string               `codec:"args"`
	BinPath       []string                  `codec:"bin_path"`
	ConfDir       string                    `codec:"conf_dir"`
	CacheDir      string                    `codec:"cache_dir"`
	Force         bool                      `codec:"force"`
}

func (cniConf CNIConfiguration) validate() error {
	if cniConf.NetworkName == "" && cniConf.NetworkConfig == nil {
		return fmt.Errorf("must specify either NetworkName or NetworkConfig in CNIConfiguration: %+v", cniConf)
	}
	if cniConf.NetworkName != "" && cniConf.NetworkConfig != nil {
		return fmt.Errorf("must not specify both NetworkName and NetworkConfig in CNIConfiguration: %+v", cniConf)
	}
	return nil
}

// NetworkInterfaces mirrors the Firecracker SDK type and is exposed in the
// task configuration.  Clients should not import the SDK directly; this type
// lets us evolve independently and add Nomad-specific helpers later.
type NetworkInterfaces []NetworkInterface

// NetworkInterface represents an interface within the microVM.
type NetworkInterface struct {
	StaticConfiguration *StaticNetworkConfiguration `codec:"static_configuration"`
	CNIConfiguration    *CNIConfiguration           `codec:"cni_configuration"`
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

// Validate is a convenience wrapper around the unexported helpers; callers
// without kernel arguments can use this method directly.
func (networkInterfaces NetworkInterfaces) Validate() error {
	return networkInterfaces.validate(nil)
}

// validate performs semantic checking of the configuration.  We replicate
// the upstream SDK's validation logic directly so that the rules remain
// visible and can be adjusted if Nomad-specific behaviour is required.
func (networkInterfaces NetworkInterfaces) validate(kernelArgs map[string]string) error {
	for _, iface := range networkInterfaces {
		hasCNI := iface.CNIConfiguration != nil
		hasStaticInterface := iface.StaticConfiguration != nil
		hasStaticIP := hasStaticInterface && iface.StaticConfiguration.IPConfiguration != nil

		if !hasCNI && !hasStaticInterface {
			return fmt.Errorf("must specify at least one of CNIConfiguration or StaticConfiguration for network interfaces: %+v", networkInterfaces)
		}

		if hasCNI && hasStaticInterface {
			return fmt.Errorf("cannot provide both CNIConfiguration and StaticConfiguration for a network interface: %+v", iface)
		}

		if hasCNI || hasStaticIP {
			if len(networkInterfaces) > 1 {
				return fmt.Errorf("cannot specify CNIConfiguration or IPConfiguration when multiple network interfaces are provided: %+v", networkInterfaces)
			}
			if argVal, ok := kernelArgs["ip"]; ok {
				return fmt.Errorf(`CNIConfiguration or IPConfiguration cannot be specified when "ip=" provided in kernel boot args, value found: "%v"`, argVal)
			}
		}

		if hasCNI {
			if err := iface.CNIConfiguration.validate(); err != nil {
				return err
			}
		}

		if hasStaticInterface {
			if err := iface.StaticConfiguration.validate(); err != nil {
				return err
			}
		}
	}
	return nil
}
