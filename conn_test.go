// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

//go:build linux

package mxremote

import (
	"net"
	"syscall"
	"testing"
	"time"
)

// A scratch group and port, deliberately not the MX Remote ones, so this never
// puts a frame on a live AV network.
const (
	testGroup = "239.255.77.99"
	testPort  = 18812

	// SO_REUSEPORT is absent from Go's syscall package; the library takes no
	// external dependencies, so the value goes in here rather than pulling in
	// golang.org/x/sys/unix for one constant.
	soReusePort = 0x0F
)

// TestMulticastFanout guards the receive path against SO_REUSEPORT.
//
// SO_REUSEADDR gives every socket bound to the same address a copy of each
// datagram. SO_REUSEPORT instead puts them in a reuseport group and hashes each
// datagram to exactly one member, multicast included - so a second MX Remote
// client on the same host would silently take half of this one's frames.
func TestMulticastFanout(t *testing.T) {
	a, err := newConn(testGroup, testPort, "", "")
	if err != nil {
		t.Skipf("no usable multicast interface: %v", err)
	}
	defer a.close()
	b, err := newConn(testGroup, testPort, "", "")
	if err != nil {
		t.Fatalf("a second client could not bind alongside the first: %v", err)
	}
	defer b.close()

	for i, c := range []*conn{a, b} {
		raw, err := c.pc.SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		var v int
		var serr error
		if err := raw.Control(func(fd uintptr) {
			v, serr = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort)
		}); err != nil {
			t.Fatal(err)
		}
		if serr != nil {
			t.Fatalf("socket %d: getsockopt SO_REUSEPORT: %v", i, serr)
		}
		if v != 0 {
			t.Fatalf("socket %d has SO_REUSEPORT set (%d): receive would be split with any other client on this host", i, v)
		}
	}

	// The delivery half needs a sender pinned to the same interface the
	// receivers joined on, with loopback on; where the host will not do that,
	// the option check above still stands on its own.
	local, err := resolveLocalIP("")
	if err != nil {
		t.Skip(err)
	}
	var localAddr [4]byte
	copy(localAddr[:], local)
	sender, err := net.ListenUDP("udp4", &net.UDPAddr{IP: local, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	raw, err := sender.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var serr error
	if err := raw.Control(func(fd uintptr) {
		if serr = ctlSetInet4(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, localAddr); serr != nil {
			return
		}
		serr = ctlSetInt(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, 1)
	}); err != nil {
		t.Fatal(err)
	}
	if serr != nil {
		t.Skipf("cannot pin the sender to %s: %v", local, serr)
	}

	const sent = 20
	counts := make(chan int, 2)
	for _, c := range []*conn{a, b} {
		go func(c *conn) {
			buf := make([]byte, 2048)
			got := 0
			for {
				_ = c.pc.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
				if _, _, err := c.read(buf); err != nil {
					counts <- got
					return
				}
				got++
			}
		}(c)
	}

	dst := &net.UDPAddr{IP: net.ParseIP(testGroup), Port: testPort}
	for i := 0; i < sent; i++ {
		if _, err := sender.WriteToUDP([]byte("ping"), dst); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	ga, gb := <-counts, <-counts
	if ga == 0 && gb == 0 {
		t.Skipf("no multicast loopback delivery via %s on this host", local)
	}
	if ga != sent || gb != sent {
		t.Fatalf("receive is partitioned rather than fanned out: %d and %d of %d sent", ga, gb, sent)
	}
}
