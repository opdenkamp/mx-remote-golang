// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

// Command discover finds MX Remote devices on the local network and prints
// them, along with their bays, as discovery completes.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	mxremote "github.com/opdenkamp/mx-remote-golang/v2"
)

func main() {
	local := flag.String("l", "", "local interface IP to bind to")
	iface := flag.String("i", "", "interface name to join multicast on (e.g. br_v8; works on no-IP VLANs)")
	bcast := flag.Bool("b", false, "use broadcast instead of multicast")
	wait := flag.Duration("w", 6*time.Second, "discovery time before printing")
	flag.Parse()

	mx := mxremote.New(mxremote.Config{
		Name:      "mxremote-go discover",
		LocalIP:   *local,
		Interface: *iface,
		Broadcast: *bcast,
		Callbacks: mxremote.Callbacks{
			OnDeviceConfigComplete: func(d *mxremote.Device) {
				fmt.Printf("ready: %s (%s) %s @ %s\n", d.Serial(), d.Name(), d.ModelName(), d.Address())
			},
		},
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := mx.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		os.Exit(1)
	}
	defer mx.Close()

	select {
	case <-ctx.Done():
	case <-time.After(*wait):
	}

	for _, d := range mx.Devices() {
		fmt.Printf("\n%s (%s) - %s - %s - %s\n", d.Serial(), d.Name(), d.ModelName(), d.Address(), d.Status())
		for _, b := range d.Outputs() {
			fmt.Printf("  [out] %-16s signal=%v video=%v\n", b.BayLabel(), b.SignalDetected(), srcLabel(b.VideoSource()))
		}
		for _, b := range d.Inputs() {
			fmt.Printf("  [in]  %-16s signal=%v %s\n", b.BayLabel(), b.SignalDetected(), streamLabel(b.V2IPSource()))
		}
	}
}

func srcLabel(b *mxremote.Bay) string {
	if b == nil {
		return "<none>"
	}
	return b.BayLabel()
}

func streamLabel(s *mxremote.V2IPStreamSources) string {
	if s == nil {
		return ""
	}
	return s.Video.String()
}
