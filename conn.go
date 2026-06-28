// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

// conn is the UDP transport: a single socket bound to the MX Remote port, joined
// to the multicast group (or broadcast-enabled), used for both transmit and
// receive. The socket is created via the net package so it works on Linux,
// macOS and Windows and integrates with the runtime poller; the multicast and
// reuse/broadcast options are applied through RawConn.Control.
type conn struct {
	pc     *net.UDPConn
	target *net.UDPAddr
}

func newConn(targetIP string, port int, localIP string) (*conn, error) {
	if localIP == "" {
		addrs := validAddresses()
		if len(addrs) == 0 {
			return nil, fmt.Errorf("failed to find a local ip address")
		}
		localIP = addrs[0]
	}
	target := net.ParseIP(targetIP).To4()
	if target == nil {
		return nil, fmt.Errorf("invalid target ip %q", targetIP)
	}
	local := net.ParseIP(localIP).To4()
	if local == nil {
		return nil, fmt.Errorf("invalid local ip %q", localIP)
	}
	var localAddr, groupAddr [4]byte
	copy(localAddr[:], local)
	copy(groupAddr[:], target)
	multicast := target[0] >= 224 && target[0] <= 239

	lc := net.ListenConfig{
		Control: func(_, _ string, rc syscall.RawConn) error {
			var sockErr error
			if err := rc.Control(func(fd uintptr) {
				if sockErr = ctlSetInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); sockErr != nil {
					return
				}
				if !multicast {
					sockErr = ctlSetInt(fd, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
				}
			}); err != nil {
				return err
			}
			return sockErr
		},
	}
	pconn, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	udp, ok := pconn.(*net.UDPConn)
	if !ok {
		pconn.Close()
		return nil, fmt.Errorf("unexpected connection type %T", pconn)
	}
	if multicast {
		if err := joinMulticast(udp, groupAddr, localAddr); err != nil {
			udp.Close()
			return nil, err
		}
	}
	return &conn{pc: udp, target: &net.UDPAddr{IP: target, Port: port}}, nil
}

// joinMulticast sets the egress interface and TTL and joins the group.
func joinMulticast(udp *net.UDPConn, group, local [4]byte) error {
	raw, err := udp.SyscallConn()
	if err != nil {
		return err
	}
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		if sockErr = ctlSetInt(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, 3); sockErr != nil {
			return
		}
		if sockErr = ctlSetInet4(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, local); sockErr != nil {
			return
		}
		sockErr = ctlSetMreq(fd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP,
			&syscall.IPMreq{Multiaddr: group, Interface: local})
	}); err != nil {
		return err
	}
	return sockErr
}

func (c *conn) transmit(data []byte) (int, error) {
	return c.pc.WriteToUDP(data, c.target)
}

func (c *conn) read(buf []byte) (int, string, error) {
	n, addr, err := c.pc.ReadFromUDP(buf)
	if err != nil {
		return 0, "", err
	}
	ip := ""
	if addr != nil {
		ip = addr.IP.String()
	}
	return n, ip, nil
}

func (c *conn) close() { c.pc.Close() }

// validAddresses returns the non-loopback IPv4 addresses of all interfaces.
func validAddresses() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
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
			out = append(out, ip.String())
		}
	}
	return out
}

// broadcastAddress returns the broadcast address for the interface matching
// localIP, or the first non-loopback IPv4 interface when localIP is empty.
func broadcastAddress(localIP string) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, ifi := range ifaces {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
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
			if localIP != "" && ip.String() != localIP {
				continue
			}
			mask := ipnet.Mask
			bcast := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				bcast[i] = ip[i] | ^mask[i]
			}
			return bcast.String(), nil
		}
	}
	return "", fmt.Errorf("no broadcast address found")
}
