// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

//go:build linux

package mxremote

import (
	"net"
	"syscall"
)

// joinMulticastIface joins the group and sets the egress interface by interface
// index. Keying on the index (rather than a local IPv4 address) means it works
// on an interface with no IPv4 address, such as a tagged VLAN.
func joinMulticastIface(udp *net.UDPConn, group [4]byte, ifi *net.Interface) error {
	raw, err := udp.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		if sockErr = ctlSetInt(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, 3); sockErr != nil {
			return
		}
		mreqn := &syscall.IPMreqn{Multiaddr: group, Ifindex: int32(ifi.Index)}
		if sockErr = syscall.SetsockoptIPMreqn(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, mreqn); sockErr != nil {
			return
		}
		sockErr = syscall.SetsockoptIPMreqn(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreqn)
	}); err != nil {
		return err
	}
	return sockErr
}
