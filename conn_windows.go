// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

//go:build windows

package mxremote

import "syscall"

// Socket-option setters used from RawConn.Control. On Windows the file
// descriptor is a syscall.Handle.

func ctlSetInt(fd uintptr, level, opt, value int) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), level, opt, value)
}

func ctlSetInet4(fd uintptr, level, opt int, addr [4]byte) error {
	return syscall.SetsockoptInet4Addr(syscall.Handle(fd), level, opt, addr)
}

func ctlSetMreq(fd uintptr, level, opt int, mreq *syscall.IPMreq) error {
	return syscall.SetsockoptIPMreq(syscall.Handle(fd), level, opt, mreq)
}
