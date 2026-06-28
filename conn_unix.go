// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

//go:build !windows

package mxremote

import "syscall"

// Socket-option setters used from RawConn.Control. On Unix the file descriptor
// is an int.

func ctlSetInt(fd uintptr, level, opt, value int) error {
	return syscall.SetsockoptInt(int(fd), level, opt, value)
}

func ctlSetInet4(fd uintptr, level, opt int, addr [4]byte) error {
	return syscall.SetsockoptInet4Addr(int(fd), level, opt, addr)
}

func ctlSetMreq(fd uintptr, level, opt int, mreq *syscall.IPMreq) error {
	return syscall.SetsockoptIPMreq(int(fd), level, opt, mreq)
}
