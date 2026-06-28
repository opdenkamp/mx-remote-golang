// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"strings"
)

//go:embed svd.csv
var svdCSV string

// Svd is a Short Video Descriptor: a standard video resolution/timing entry.
type Svd struct {
	ID               int
	PictureAspect    int
	PixelAspect      int
	HorizontalActive int
	HorizontalTotal  int
	VerticalActive   int
	VerticalTotal    int
	Refresh          int
	Interlaced       bool
	Multiplier       int
}

func (s Svd) String() string {
	return fmt.Sprintf("%dx%d@%dHz", s.HorizontalActive, s.VerticalActive, s.Refresh)
}

var svdMap = loadSvd()

func loadSvd() map[int]Svd {
	m := map[int]Svd{}
	for _, line := range strings.Split(svdCSV, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, ";")
		if len(f) < 10 {
			continue
		}
		n := make([]int, 10)
		ok := true
		for i := 0; i < 10; i++ {
			v, err := strconv.Atoi(strings.TrimSpace(f[i]))
			if err != nil {
				ok = false
				break
			}
			n[i] = v
		}
		if !ok {
			continue
		}
		m[n[0]] = Svd{
			ID: n[0], PictureAspect: n[1], PixelAspect: n[2],
			HorizontalActive: n[3], HorizontalTotal: n[4],
			VerticalActive: n[5], VerticalTotal: n[6],
			Refresh: n[7], Interlaced: n[8] == 1, Multiplier: n[9],
		}
	}
	return m
}

// LookupSvd returns the Short Video Descriptor for the given id.
func LookupSvd(id int) (Svd, bool) {
	s, ok := svdMap[id]
	return s, ok
}

func colourSpaceString(v uint8) string {
	switch v {
	case 0:
		return "RGB"
	case 1:
		return "4:4:4"
	case 2:
		return "4:2:2"
	case 3:
		return "4:2:0"
	}
	return "unknown"
}

// handleSignalStatusNew (opcode 0x31) decodes the detailed AV signal report and
// updates the bay's signal-detected flag and signal-type description.
func handleSignalStatusNew(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 112 {
		return
	}
	supportFlags := p[2]
	streamFlags := p[3]
	streamValid := supportFlags&(1<<1) != 0

	portNum := int(p[100]) | int(p[101])<<8
	bay := dev.getByPortnumLocked(portNum)
	if bay == nil {
		return
	}

	video := p[40:56]
	svdID := int(video[0])
	var signalType string
	if streamValid {
		if svd, ok := svdMap[svdID]; ok && svdID != 0 {
			rate := float64(video[8])
			if streamFlags&(1<<3) != 0 { // non-integer clock
				rate = math.Round(rate*1000/1001*100) / 100
			}
			signalType = fmt.Sprintf("%dx%d / %s / %dbpp",
				svd.HorizontalActive, svd.VerticalActive, colourSpaceString(video[1]), video[2])
			if streamFlags&(1<<1) != 0 { // interlaced
				signalType += " interlaced"
			}
			if streamFlags&(1<<4) != 0 { // HDR
				signalType += " HDR"
			}
			signalType += fmt.Sprintf(" / %sHz", strconv.FormatFloat(rate, 'f', -1, 64))
		} else {
			signalType = "No Signal"
		}
	} else {
		signalType = "No Signal"
	}

	bay.setBoolStatus(&bay.signalDetected, streamValid, r.callbacks.OnStatusSignalDetectedChanged, false)
	bay.setSignalType(signalType)
}
