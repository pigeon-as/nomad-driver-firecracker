// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package network

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"syscall"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// GuestNetworkConfig holds a single IP configuration the guest needs to apply
// to its eth0 interface. In dual-stack CNI setups, AutoSetup returns multiple
// configs (one per address family). This is read from the veth inside the
// network namespace during AutoSetup and passed to the driver so it can inject
// the values into MMDS for pigeon-init to consume.
//
// The field names and JSON tags match pigeon-init's config.IPConfig struct
// so the driver can inject this directly into the MMDS RunConfig.
type GuestNetworkConfig struct {
	// IP is the address without prefix, e.g. "172.26.64.2" or "fdaa:a1b2::5".
	IP string `json:"IP"`
	// Mask is the prefix length, e.g. 20 for IPv4 or 64 for IPv6.
	Mask int `json:"Mask"`
	// Gateway is the default gateway IP, e.g. "172.26.64.1".
	// May be empty for IPv6 link-local routes.
	Gateway string `json:"Gateway"`
}

const (
	// TapName is the name of the TAP device created inside the network namespace.
	// Each allocation gets its own namespace in bridge mode, so "tap0" is
	// unambiguous — provided only one Firecracker task runs per group.
	// Multiple Firecracker tasks in the same group are unsupported.
	TapName = "tap0"

	// ethPAll matches all protocols for TC filters (linux/if_ether.h ETH_P_ALL).
	ethPAll = 0x0003
)

// AutoSetup creates a TAP device with TC redirect inside the given network
// namespace and returns a single-element NetworkInterfaces configured to use
// it, along with the guest network configurations read from the veth.
//
// This is the standard path for Nomad bridge/CNI networking: the TAP device
// bridges VM traffic through the veth created by Nomad. The returned
// GuestNetworkConfigs contain the IP/mask/gateway entries that the guest init
// process must apply to its eth0 to participate in the network.
//
// When CNI IPAM assigns dual-stack addresses (IPv4 + IPv6), both are returned.
func AutoSetup(netnsPath string) (NetworkInterfaces, []GuestNetworkConfig, error) {
	tapName, guestNets, err := SetupTapRedirect(netnsPath)
	if err != nil {
		return nil, nil, err
	}
	return NetworkInterfaces{
		{
			StaticConfiguration: &StaticNetworkConfiguration{
				HostDevName: tapName,
			},
		},
	}, guestNets, nil
}

// SetupTapRedirect creates a TAP device inside the given network namespace and
// sets up bidirectional TC redirect filters between the existing veth interface
// (created by Nomad bridge mode) and the new TAP device.
//
// This implements the same mechanism as the tc-redirect-tap CNI plugin
// (https://github.com/awslabs/tc-redirect-tap). Traffic flows:
//
//	VM → TAP → (TC redirect) → veth → Nomad bridge → host (and back)
//
// It also reads the IP configuration from the veth so the driver can pass it
// to the guest via MMDS.
//
// The caller should pass the returned TAP name as host_dev_name in the
// Firecracker VM configuration.
func SetupTapRedirect(netnsPath string) (string, []GuestNetworkConfig, error) {
	// Lock the goroutine to the OS thread so the namespace switch doesn't
	// leak to other goroutines via the Go scheduler.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origNS, err := netns.Get()
	if err != nil {
		return "", nil, fmt.Errorf("get current netns: %w", err)
	}
	defer origNS.Close()

	targetNS, err := netns.GetFromPath(netnsPath)
	if err != nil {
		return "", nil, fmt.Errorf("open netns %q: %w", netnsPath, err)
	}
	defer targetNS.Close()

	if err := netns.Set(targetNS); err != nil {
		return "", nil, fmt.Errorf("enter netns: %w", err)
	}
	defer netns.Set(origNS) //nolint:errcheck

	// Find the veth that Nomad created inside the namespace.
	veth, err := findVeth()
	if err != nil {
		return "", nil, err
	}

	// Read all IP configurations from the veth before creating the TAP.
	// The guest needs these to configure its own eth0 interface.
	// With dual-stack CNI IPAM, both IPv4 and IPv6 addresses are returned.
	guestNets, err := readVethConfigs(veth)
	if err != nil {
		return "", nil, err
	}

	tap, err := createTap(TapName, veth.Attrs().MTU)
	if err != nil {
		return "", nil, err
	}

	if err := addRedirects(veth, tap); err != nil {
		_ = netlink.LinkDel(tap)
		return "", nil, err
	}

	return TapName, guestNets, nil
}

// findVeth returns the veth link in the current network namespace.
// In a Nomad bridge namespace there is exactly one: the veth peer created by
// the bridge network mutator. Filtering by type avoids accidentally selecting
// a TAP device that may already exist from a previous task attempt.
func findVeth() (netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	for _, l := range links {
		if l.Type() == "veth" {
			return l, nil
		}
	}
	return nil, fmt.Errorf("no veth interface found in network namespace")
}

// createTap creates and brings up a TAP device with the given name and MTU.
// If the TAP already exists (e.g. task restart within the same allocation),
// it is reused. Firecracker does not support multiqueue tap devices, so
// Queues is set to 1.
func createTap(name string, mtu int) (netlink.Link, error) {
	attrs := netlink.NewLinkAttrs()
	attrs.Name = name

	tap := &netlink.Tuntap{
		LinkAttrs: attrs,
		Mode:      netlink.TUNTAP_MODE_TAP,
		Queues:    1,
		Flags:     netlink.TUNTAP_ONE_QUEUE | netlink.TUNTAP_VNET_HDR,
	}

	if err := netlink.LinkAdd(tap); err != nil {
		if !errors.Is(err, syscall.EEXIST) {
			return nil, fmt.Errorf("create tap device: %w", err)
		}
		// TAP already exists from a previous task attempt — reuse it
		// after ensuring MTU and link state are correct.
		link, lookupErr := netlink.LinkByName(name)
		if lookupErr != nil {
			return nil, fmt.Errorf("find existing tap %q: %w", name, lookupErr)
		}
		if err := netlink.LinkSetMTU(link, mtu); err != nil {
			return nil, fmt.Errorf("set existing tap %q MTU to %d: %w", name, mtu, err)
		}
		if err := netlink.LinkSetUp(link); err != nil {
			return nil, fmt.Errorf("bring existing tap %q up: %w", name, err)
		}
		return link, nil
	}

	// Close the file descriptors that netlink opened on /dev/net/tun.
	// Firecracker needs to open the TAP device itself; if these FDs stay
	// open the device is "busy" and Firecracker's TUNSETIFF ioctl fails
	// with EBUSY.
	for _, fd := range tap.Fds {
		if fd != nil {
			fd.Close()
		}
	}

	if err := netlink.LinkSetMTU(tap, mtu); err != nil {
		_ = netlink.LinkDel(tap)
		return nil, fmt.Errorf("set tap MTU: %w", err)
	}

	if err := netlink.LinkSetUp(tap); err != nil {
		_ = netlink.LinkDel(tap)
		return nil, fmt.Errorf("bring tap up: %w", err)
	}

	// Re-fetch to get updated attributes (index, flags, etc.).
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("find created tap: %w", err)
	}

	return link, nil
}

// addRedirects adds ingress qdiscs and bidirectional u32 redirect filters
// between the veth and TAP devices, replicating tc-redirect-tap behavior.
func addRedirects(veth, tap netlink.Link) error {
	// Add ingress qdiscs to both interfaces.
	for _, link := range []netlink.Link{veth, tap} {
		qdisc := &netlink.Ingress{
			QdiscAttrs: netlink.QdiscAttrs{
				LinkIndex: link.Attrs().Index,
				Parent:    netlink.HANDLE_INGRESS,
			},
		}
		if err := netlink.QdiscAdd(qdisc); err != nil {
			if !errors.Is(err, syscall.EEXIST) {
				return fmt.Errorf("add ingress qdisc to %s: %w", link.Attrs().Name, err)
			}
		}
	}

	// Add bidirectional redirect filters.
	rootHandle := netlink.MakeHandle(0xffff, 0)
	for _, pair := range [][2]netlink.Link{{veth, tap}, {tap, veth}} {
		if err := netlink.FilterAdd(&netlink.U32{
			FilterAttrs: netlink.FilterAttrs{
				LinkIndex: pair[0].Attrs().Index,
				Parent:    rootHandle,
				Protocol:  ethPAll,
			},
			Actions: []netlink.Action{
				&netlink.MirredAction{
					ActionAttrs: netlink.ActionAttrs{
						Action: netlink.TC_ACT_STOLEN,
					},
					MirredAction: netlink.TCA_EGRESS_REDIR,
					Ifindex:      pair[1].Attrs().Index,
				},
			},
		}); err != nil {
			if !errors.Is(err, syscall.EEXIST) {
				return fmt.Errorf("add redirect %s → %s: %w",
					pair[0].Attrs().Name, pair[1].Attrs().Name, err)
			}
		}
	}

	return nil
}

// readVethConfigs reads all IP addresses (IPv4 and IPv6) and their associated
// gateways from the veth link in the current network namespace. This must be
// called while the goroutine is in the target namespace.
//
// With dual-stack CNI IPAM (e.g. host-local with both IPv4 and IPv6 ranges),
// this returns one GuestNetworkConfig per address. IPv6 link-local addresses
// (fe80::/10) are excluded — only global/ULA addresses are returned.
//
// At least one address must exist or an error is returned.
func readVethConfigs(veth netlink.Link) ([]GuestNetworkConfig, error) {
	var configs []GuestNetworkConfig

	// Read IPv4 addresses.
	v4Addrs, err := netlink.AddrList(veth, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("list IPv4 addresses on %s: %w", veth.Attrs().Name, err)
	}

	v4Gateway := findGateway(veth, netlink.FAMILY_V4)

	for _, addr := range v4Addrs {
		if addr.IPNet == nil {
			continue
		}
		ones, _ := addr.Mask.Size()
		configs = append(configs, GuestNetworkConfig{
			IP:      addr.IP.String(),
			Mask:    ones,
			Gateway: v4Gateway,
		})
	}

	// Read IPv6 addresses (skip link-local fe80::/10).
	v6Addrs, err := netlink.AddrList(veth, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("list IPv6 addresses on %s: %w", veth.Attrs().Name, err)
	}

	v6Gateway := findGateway(veth, netlink.FAMILY_V6)

	for _, addr := range v6Addrs {
		if addr.IPNet == nil {
			continue
		}
		// Skip link-local addresses — the guest doesn't need them.
		if addr.IP.IsLinkLocalUnicast() {
			continue
		}
		ones, _ := addr.Mask.Size()
		configs = append(configs, GuestNetworkConfig{
			IP:      addr.IP.String(),
			Mask:    ones,
			Gateway: v6Gateway,
		})
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no usable IP addresses on %s", veth.Attrs().Name)
	}

	return configs, nil
}

// findGateway looks for a default gateway in the routing table for the given
// address family. Returns empty string if none found.
func findGateway(veth netlink.Link, family int) string {
	routes, err := netlink.RouteList(veth, family)
	if err != nil {
		return ""
	}
	for _, r := range routes {
		if r.Dst == nil || r.Dst.IP.Equal(net.IPv4zero) || r.Dst.IP.Equal(net.IPv6zero) {
			if r.Gw != nil {
				return r.Gw.String()
			}
		}
	}

	// Also check routes not bound to a specific link.
	allRoutes, err := netlink.RouteList(nil, family)
	if err != nil {
		return ""
	}
	for _, r := range allRoutes {
		if r.Dst == nil || r.Dst.IP.Equal(net.IPv4zero) || r.Dst.IP.Equal(net.IPv6zero) {
			if r.Gw != nil {
				return r.Gw.String()
			}
		}
	}

	return ""
}
