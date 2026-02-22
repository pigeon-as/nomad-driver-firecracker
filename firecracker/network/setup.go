// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package network

import (
	"fmt"
	"runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

const (
	// TapName is the name of the TAP device created inside the network namespace.
	// Each VM gets its own namespace, so "tap0" is unambiguous.
	TapName = "tap0"

	// ethPAll matches all protocols for TC filters (linux/if_ether.h ETH_P_ALL).
	ethPAll = 0x0003
)

// SetupTapRedirect creates a TAP device inside the given network namespace and
// sets up bidirectional TC redirect filters between the existing veth interface
// (created by Nomad bridge mode) and the new TAP device.
//
// This implements the same mechanism as the tc-redirect-tap CNI plugin
// (https://github.com/awslabs/tc-redirect-tap). Traffic flows:
//
//	VM → TAP → (TC redirect) → veth → Nomad bridge → host (and back)
//
// The caller should pass the returned TAP name as host_dev_name in the
// Firecracker VM configuration.
func SetupTapRedirect(netnsPath string) (string, error) {
	// Lock the goroutine to the OS thread so the namespace switch doesn't
	// leak to other goroutines via the Go scheduler.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origNS, err := netns.Get()
	if err != nil {
		return "", fmt.Errorf("get current netns: %w", err)
	}
	defer origNS.Close()

	targetNS, err := netns.GetFromPath(netnsPath)
	if err != nil {
		return "", fmt.Errorf("open netns %q: %w", netnsPath, err)
	}
	defer targetNS.Close()

	if err := netns.Set(targetNS); err != nil {
		return "", fmt.Errorf("enter netns: %w", err)
	}
	defer netns.Set(origNS) //nolint:errcheck

	// Find the veth that Nomad created inside the namespace.
	veth, err := findVeth()
	if err != nil {
		return "", err
	}

	tap, err := createTap(TapName, veth.Attrs().MTU)
	if err != nil {
		return "", err
	}

	if err := addRedirects(veth, tap); err != nil {
		_ = netlink.LinkDel(tap)
		return "", err
	}

	return TapName, nil
}

// findVeth returns the first non-loopback link in the current network namespace.
func findVeth() (netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	for _, l := range links {
		if l.Attrs().Name != "lo" {
			return l, nil
		}
	}
	return nil, fmt.Errorf("no non-loopback interface found in network namespace")
}

// createTap creates and brings up a TAP device with the given name and MTU.
// Firecracker does not support multiqueue tap devices, so Queues is set to 1.
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
		return nil, fmt.Errorf("create tap device: %w", err)
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
			return fmt.Errorf("add ingress qdisc to %s: %w", link.Attrs().Name, err)
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
			return fmt.Errorf("add redirect %s → %s: %w",
				pair[0].Attrs().Name, pair[1].Attrs().Name, err)
		}
	}

	return nil
}
