// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

// Package mxremote is a Go library for interfacing with Pulse-Eight MX Remote
// compatible devices over a local network (UDP multicast or broadcast): video/
// audio matrices, OneIP/V2IP units (transmitter/receiver/transceiver/
// multiviewer) and amplifiers, all running the shared MatrixOS firmware.
//
// The wire protocol is byte-for-byte compatible with the reference Python
// library and the MatrixOS firmware. The Go API itself is idiomatic and does
// not mirror the Python class layout.
//
// Typical use:
//
//	mx := mxremote.New(mxremote.Config{
//		Callbacks: mxremote.Callbacks{
//			OnDeviceConfigComplete: func(d *mxremote.Device) {
//				fmt.Println("ready:", d.Serial(), d.ModelName())
//			},
//		},
//	})
//	if err := mx.Start(ctx); err != nil {
//		log.Fatal(err)
//	}
//	defer mx.Close()
//
// Discovery runs in the background; devices and their bays are populated as
// hello and configuration frames arrive. Callbacks fire from a single internal
// goroutine. V2IP routing is driven from sink bays via Bay.SelectVideoSource,
// Bay.SelectAudioSource and Bay.SelectAudioSourceAddr.
package mxremote
