// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	_ "embed"
	"encoding/binary"
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

// av_details wire layout (packed):
//
//	0..8     av_details_header
//	8..24    avi_infoframe
//	24..40   av_details_audio
//	40..56   av_details_video
//	56..88   av_details_vsync
//	88..100  av_details_hdmi_link_errors (3 x u32)
//	100..112 av_details_bay
const avDetailsSize = 112

// handleSignalStatusNew (opcode 0x31) decodes the detailed AV signal report and
// updates the bay's signal-detected flag, signal-type description and details.
//
// A report is answered one packet per bay, not one per device: the port number
// in the bay block at the tail is what names the reporting bay, so demultiplex
// on it. Because that block sits behind the vsync and link-error tail, a report
// shorter than the full struct cannot be attributed to a bay at all and is
// dropped - firmware does the same.
//
// An empty payload is a broadcast request for every device to report; a 16-byte
// payload requests a report from the one unit it addresses.
func handleSignalStatusNew(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < avDetailsSize {
		return
	}
	supportFlags := p[2]
	streamFlags := p[3]
	streamValid := supportFlags&(1<<1) != 0

	bayBlock := p[100:112]
	portNum := int(binary.LittleEndian.Uint16(bayBlock[0:2]))
	bay := dev.getByPortnumLocked(portNum)
	if bay == nil {
		return
	}

	video := p[40:56]
	svdID := int(video[0])
	frameRate := float64(binary.LittleEndian.Uint16(video[8:10]))
	if streamFlags&(1<<3) != 0 { // non-integer clock
		frameRate = math.Round(frameRate*1000/1001*100) / 100
	}

	var signalType string
	if svd, ok := svdMap[svdID]; streamValid && ok && svdID != 0 {
		signalType = fmt.Sprintf("%dx%d / %s / %dbpp",
			svd.HorizontalActive, svd.VerticalActive, colourSpaceString(video[1]), video[2])
		if streamFlags&(1<<1) != 0 { // interlaced
			signalType += " interlaced"
		}
		if streamFlags&(1<<4) != 0 { // HDR
			signalType += " HDR"
		}
		signalType += fmt.Sprintf(" / %sHz", strconv.FormatFloat(frameRate, 'f', -1, 64))
	} else {
		signalType = "No Signal"
	}

	bay.setSignalDetails(BaySignalDetails{
		FrameRate: frameRate,
		TmdsClock: binary.LittleEndian.Uint32(video[10:14]),
		Status:    BayStatusMask(binary.LittleEndian.Uint32(bayBlock[2:6])),
		Scaling:   MxrSignalType(binary.LittleEndian.Uint16(bayBlock[6:8])),
		ClockRate: binary.LittleEndian.Uint32(bayBlock[8:12]),
	})
	bay.setBoolStatus(&bay.signalDetected, streamValid, r.callbacks.OnStatusSignalDetectedChanged, false)
	bay.setSignalType(signalType)
}
