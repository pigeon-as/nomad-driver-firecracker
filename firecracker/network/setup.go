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

// GuestNetworkConfig holds the IP configuration the guest needs to apply to
// its eth0 interface. This is read from the veth inside the network namespace
// during AutoSetup and passed to the driver so it can inject the values into
// MMDS for pigeon-init to consume.
//
// The field names and JSON tags match pigeon-init's config.IPConfig struct
// so the driver can inject this directly into the MMDS RunConfig.
type GuestNetworkConfig struct {
	// IP is the IPv4 address without prefix, e.g. "172.26.64.2".
	IP string `json:"IP"`
	// Mask is the prefix length, e.g. 20.
	Mask int `json:"Mask"`
	// Gateway is the default gateway IP, e.g. "172.26.64.1".
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
// it, along with the guest network configuration read from the veth.
//
// This is the standard path for Nomad bridge networking: the TAP device
// bridges VM traffic through the veth created by Nomad. The returned
// GuestNetworkConfig contains the IP/mask/gateway that the guest init
// process must apply to its eth0 to participate in the network.
func AutoSetup(netnsPath string) (NetworkInterfaces, *GuestNetworkConfig, error) {
	tapName, guestNet, err := SetupTapRedirect(netnsPath)
	if err != nil {
		return nil, nil, err
	}
	return NetworkInterfaces{
		{
			StaticConfiguration: &StaticNetworkConfiguration{
				HostDevName: tapName,
			},
		},
	}, guestNet, nil
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
func SetupTapRedirect(netnsPath string) (string, *GuestNetworkConfig, error) {
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

	// Read the veth IP configuration before creating the TAP. The guest
	// needs this to configure its own eth0 interface.
	guestNet, err := readVethConfig(veth)
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

	return TapName, guestNet, nil
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

// readVethConfig reads the IPv4 address, prefix length, and default gateway
// from the veth link in the current network namespace. This must be called
// while the goroutine is in the target namespace.
func readVethConfig(veth netlink.Link) (*GuestNetworkConfig, error) {
	addrs, err := netlink.AddrList(veth, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("list addresses on %s: %w", veth.Attrs().Name, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no IPv4 address on %s", veth.Attrs().Name)
	}

	addr := addrs[0]
	if addr.IPNet == nil {
		return nil, fmt.Errorf("address on %s has no IPNet", veth.Attrs().Name)
	}
	ones, _ := addr.Mask.Size()

	// Find the default gateway from the routing table.
	routes, err := netlink.RouteList(veth, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("list routes on %s: %w", veth.Attrs().Name, err)
	}

	var gateway string
	for _, r := range routes {
		if r.Dst == nil || r.Dst.IP.Equal(net.IPv4zero) {
			if r.Gw != nil {
				gateway = r.Gw.String()
				break
			}
		}
	}

	// Also check routes not bound to a specific link (default route).
	if gateway == "" {
		allRoutes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err == nil {
			for _, r := range allRoutes {
				if r.Dst == nil || r.Dst.IP.Equal(net.IPv4zero) {
					if r.Gw != nil {
						gateway = r.Gw.String()
						break
					}
				}
			}
		}
	}

	if gateway == "" {
		return nil, fmt.Errorf("no default gateway found for %s", veth.Attrs().Name)
	}

	return &GuestNetworkConfig{
		IP:      addr.IP.String(),
		Mask:    ones,
		Gateway: gateway,
	}, nil
}
