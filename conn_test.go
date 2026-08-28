// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

//go:build linux

package mxremote

import (
	"encoding/binary"
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

// The last layer: that a control method's frame actually reaches the socket.
//
// The transmit tap the round-trip tests use sits inside transmit, after the
// protocol gate and before the write, so it proves what would be sent and not
// that anything was. Between the tap and the wire sit a nil check and the
// socket call, and "reported success for a frame never written" is a real
// failure — the Python port had exactly that, from a length comparison that
// could never match.
//
// The host's multicast loopback is proved with a probe first, deliberately. An
// earlier version skipped when nothing arrived, which meant a library that sent
// nothing produced a skip rather than a failure — the test could not fail in
// the direction it was looking. Once the probe lands, silence is the library's.
func TestControlMethodReachesTheSocket(t *testing.T) {
	const group, port = "239.255.77.98", 18813

	sender, err := newConn(group, port, "", "")
	if err != nil {
		t.Skipf("no usable multicast interface: %v", err)
	}
	defer sender.close()
	listener, err := newConn(group, port, "", "")
	if err != nil {
		t.Fatalf("listener could not bind alongside: %v", err)
	}
	defer listener.close()

	read := func(d time.Duration) []byte {
		buf := make([]byte, 2048)
		if err := listener.pc.SetReadDeadline(time.Now().Add(d)); err != nil {
			return nil
		}
		n, _, err := listener.read(buf)
		if err != nil {
			return nil
		}
		return append([]byte(nil), buf[:n]...)
	}

	// does loopback work here at all? if not, skip for the host's reasons
	probe := buildFrame(uidN(212), opSysDiscover, protocolFor(opSysDiscover), nil)
	if _, err := sender.transmit(probe); err != nil {
		t.Skipf("cannot transmit on this host: %v", err)
	}
	if read(2*time.Second) == nil {
		t.Skip("no multicast loopback on this host, so a missing frame would be ambiguous")
	}

	r := newTestRemote(Callbacks{})
	r.uid = uidN(210)
	peer := uidN(211)
	feed := feeder(r, peer)
	feed(opSysHello, helloPayload(0x28, "FF88", "SK0001", "4.8.0", FeatureVideoRouting))
	feed(opSysBayConfig, bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut))

	r.mu.Lock()
	r.conn = sender
	r.mu.Unlock()

	if err := r.GetByUID(peer).GetByPortnum(2).SetName("Kitchen"); err != nil {
		t.Fatalf("send returned an error: %v", err)
	}

	got := read(2 * time.Second)
	if got == nil {
		t.Fatal("the method reported success but nothing reached the socket")
	}
	if len(got) < headerLen {
		t.Fatalf("received %d bytes, too short for a frame", len(got))
	}
	if op := binary.LittleEndian.Uint16(got[20:22]); op != opChangeBayName {
		t.Fatalf("opcode on the wire = %#02x, want %#02x", op, opChangeBayName)
	}
	// and it is the frame the method meant to send, not merely some frame
	if name := cstr(got[headerLen+18 : headerLen+18+deviceNameLen]); name != "Kitchen" {
		t.Fatalf("name on the wire = %q, want %q", name, "Kitchen")
	}
}
