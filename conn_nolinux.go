// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

//go:build !linux

package mxremote

import (
	"fmt"
	"net"
)

// joinMulticastIface falls back to an IP-keyed join using the named interface's
// first non-loopback IPv4 address. The interface-index (no-IP) path is Linux
// only, so on macOS and Windows an interface without an IPv4 address is an error.
func joinMulticastIface(udp *net.UDPConn, group [4]byte, ifi *net.Interface) error {
	addrs, err := ifi.Addrs()
	if err != nil {
		return err
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() {
			continue
		}
		var local [4]byte
		copy(local[:], ip)
		return joinMulticast(udp, group, local)
	}
	return fmt.Errorf("interface %q has no usable IPv4 address (interface-index join is Linux-only)", ifi.Name)
}
