// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import "fmt"

// AudioFeatures is the capability bitmask of a V2IP audio endpoint.
type AudioFeatures uint32

const (
	audioFeatureInput       AudioFeatures = 1 << 0
	audioFeatureOutput      AudioFeatures = 1 << 1
	audioFeatureV2IPTx      AudioFeatures = 1 << 2
	audioFeatureV2IPRx      AudioFeatures = 1 << 3
	audioFeatureHDMI        AudioFeatures = 1 << 4
	audioFeatureRCA         AudioFeatures = 1 << 5
	audioFeatureSPDIF       AudioFeatures = 1 << 6
	audioFeatureTrigger     AudioFeatures = 1 << 7
	audioFeatureMute        AudioFeatures = 1 << 8
	audioFeatureRouteInput  AudioFeatures = 1 << 9
	audioFeatureRouteOutput AudioFeatures = 1 << 10
	audioFeatureRouteInNone AudioFeatures = 1 << 11
	audioFeatureAmpOutput   AudioFeatures = 1 << 12
	audioFeatureVolumeCtrl  AudioFeatures = 1 << 13
	audioFeatureGainCtrl    AudioFeatures = 1 << 14
)

func (a AudioFeatures) IsInput() bool       { return a&audioFeatureInput != 0 }
func (a AudioFeatures) IsOutput() bool      { return a&audioFeatureOutput != 0 }
func (a AudioFeatures) IsV2IPTx() bool      { return a&audioFeatureV2IPTx != 0 }
func (a AudioFeatures) IsV2IPRx() bool      { return a&audioFeatureV2IPRx != 0 }
func (a AudioFeatures) IsHDMI() bool        { return a&audioFeatureHDMI != 0 }
func (a AudioFeatures) IsRCA() bool         { return a&audioFeatureRCA != 0 }
func (a AudioFeatures) IsSPDIF() bool       { return a&audioFeatureSPDIF != 0 }
func (a AudioFeatures) HasTrigger() bool    { return a&audioFeatureTrigger != 0 }
func (a AudioFeatures) SupportMute() bool   { return a&audioFeatureMute != 0 }
func (a AudioFeatures) IsAmp() bool         { return a&audioFeatureAmpOutput != 0 }
func (a AudioFeatures) SupportVolume() bool { return a&audioFeatureVolumeCtrl != 0 }
func (a AudioFeatures) IsV2IP() bool        { return a.IsV2IPRx() || a.IsV2IPTx() }

// AudioEndpoint is a single audio endpoint on a V2IP device (an input, output,
// or processor node in the device's audio tree).
type AudioEndpoint struct {
	ID       int
	Features AudioFeatures
	Address  *V2IPStreamSource

	parentID     *int
	inRoutesSupp *uint32
	inRoutes     *uint32
	children     []*AudioEndpoint
	parent       *AudioEndpoint
	bay          *Bay
	linkedUID    DeviceUID
	linkedEP     *int
	container    *AudioEndpoints
}

func (e *AudioEndpoint) IsV2IP() bool   { return e.Features.IsV2IP() }
func (e *AudioEndpoint) IsInput() bool  { return e.Features.IsInput() }
func (e *AudioEndpoint) IsOutput() bool { return e.Features.IsOutput() }

// Bay returns the bay this endpoint is associated with, or nil.
func (e *AudioEndpoint) Bay() *Bay { return e.bay }

// Input returns the currently routed input endpoint, or nil.
func (e *AudioEndpoint) Input() *AudioEndpoint {
	if e.inRoutes == nil {
		return nil
	}
	for id := 0; id < 32; id++ {
		if *e.inRoutes&(1<<uint(id)) != 0 {
			return e.container.Get(id)
		}
	}
	return nil
}

// InputsAvailable returns the input endpoints that can be routed to this one.
func (e *AudioEndpoint) InputsAvailable() []*AudioEndpoint {
	var rv []*AudioEndpoint
	if e.inRoutesSupp == nil {
		return rv
	}
	for id := 0; id < 32; id++ {
		if *e.inRoutesSupp&(1<<uint(id)) != 0 {
			if ep := e.container.Get(id); ep != nil {
				rv = append(rv, ep)
			}
		}
	}
	return rv
}

func (e *AudioEndpoint) setBay(b *Bay) {
	e.bay = b
	for _, c := range e.children {
		c.setBay(b)
	}
}

// AudioEndpoints is the collection of audio endpoints reported by a device.
type AudioEndpoints struct {
	endpoints map[int]*AudioEndpoint
	order     []int
}

func newAudioEndpoints() *AudioEndpoints {
	return &AudioEndpoints{endpoints: map[int]*AudioEndpoint{}}
}

func (a *AudioEndpoints) add(ep *AudioEndpoint) {
	if _, ok := a.endpoints[ep.ID]; !ok {
		a.order = append(a.order, ep.ID)
	}
	a.endpoints[ep.ID] = ep
}

// Get returns the endpoint with the given id, or nil.
func (a *AudioEndpoints) Get(id int) *AudioEndpoint {
	if a == nil {
		return nil
	}
	return a.endpoints[id]
}

// List returns all endpoints in wire order.
func (a *AudioEndpoints) List() []*AudioEndpoint {
	var rv []*AudioEndpoint
	for _, id := range a.order {
		rv = append(rv, a.endpoints[id])
	}
	return rv
}

func (a *AudioEndpoints) tree() []*AudioEndpoint {
	var rv []*AudioEndpoint
	for _, id := range a.order {
		if a.endpoints[id].parent == nil {
			rv = append(rv, a.endpoints[id])
		}
	}
	return rv
}

func (a *AudioEndpoints) treeFirstInput() *AudioEndpoint {
	for _, ep := range a.tree() {
		if ep.IsInput() {
			return ep
		}
	}
	return nil
}

func (a *AudioEndpoints) treeFirstOutput() *AudioEndpoint {
	for _, ep := range a.tree() {
		if ep.IsOutput() {
			return ep
		}
	}
	return nil
}

func (a *AudioEndpoints) equal(b *AudioEndpoints) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.endpoints) != len(b.endpoints) {
		return false
	}
	for id, ep := range a.endpoints {
		o, ok := b.endpoints[id]
		if !ok || o.Features != ep.Features || !sameIntPtr(o.parentID, ep.parentID) {
			return false
		}
	}
	return true
}

func sameIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// AudioLink links a local audio endpoint to an endpoint on another device.
type AudioLink struct {
	Endpoint     int
	LinkEndpoint int
	LinkDevice   DeviceUID
}

// AudioChangeSource describes an audio input-selection change: which source
// endpoint a sink endpoint was switched to.
//
// The sink is named twice on the wire, once as the command header's target and
// again at the head of the body; the body's second uid is the source. The
// reference Python library's decoder reads these the other way round from its
// own frame builder - the header convention every other audio sub-command
// follows is what settles it.
type AudioChangeSource struct {
	// SourceUID and SourceID name the endpoint being listened to.
	SourceUID DeviceUID
	SourceID  int
	// TargetUID and TargetID name the sink endpoint doing the listening.
	TargetUID DeviceUID
	TargetID  int
}

func (a AudioChangeSource) String() string {
	return fmt.Sprintf("audio source change %s:%d -> %s:%d",
		a.SourceUID, a.SourceID, a.TargetUID, a.TargetID)
}

// parseAudioChangeSource decodes a SELECT_INPUT body, which follows the 20-byte
// audio command header: sink uid, source uid, then the two endpoint ids.
func parseAudioChangeSource(f *frame) (AudioChangeSource, bool) {
	sink, ok := f.uuid(20)
	if !ok {
		return AudioChangeSource{}, false
	}
	source, ok := f.uuid(36)
	if !ok {
		return AudioChangeSource{}, false
	}
	sinkEP, ok := f.u16(52)
	if !ok {
		return AudioChangeSource{}, false
	}
	sourceEP, ok := f.u16(54)
	if !ok {
		return AudioChangeSource{}, false
	}
	return AudioChangeSource{
		SourceUID: source, SourceID: int(sourceEP),
		TargetUID: sink, TargetID: int(sinkEP),
	}, true
}

// audio command opcodes (u16 at payload offset 0).
const (
	audioOpFeatures    = 0
	audioOpMute        = 1
	audioOpTrigger     = 2
	audioOpSelectInput = 3
	audioOpVolume      = 4
	audioOpLinks       = 5
)

const (
	audioEntryEndpoint = 1
	audioEntryAddress  = 2
	audioEntryRouteIn  = 3
	audioEntryParent   = 5
)

// parseAudioEndpoints decodes the FEATURES body into an endpoint tree.
func parseAudioEndpoints(f *frame) *AudioEndpoints {
	eps := newAudioEndpoints()
	nb, ok := f.u16(28)
	if !ok {
		return eps
	}
	entryIdx := func(x int) int { return 36 + x*16 }

	for x := 0; x < int(nb); x++ {
		base := entryIdx(x)
		id, ok := f.u8(base)
		if !ok {
			break
		}
		etype, _ := f.u8(base + 1)
		if etype == audioEntryEndpoint {
			feat, _ := f.u32(base + 8)
			eps.add(&AudioEndpoint{ID: int(id), Features: AudioFeatures(feat), container: eps})
		}
	}
	for x := 0; x < int(nb); x++ {
		base := entryIdx(x)
		id, ok := f.u8(base)
		if !ok {
			break
		}
		ep := eps.Get(int(id))
		if ep == nil {
			continue
		}
		etype, _ := f.u8(base + 1)
		switch etype {
		case audioEntryAddress:
			if b := f.bytesFrom(base + 8); len(b) >= 6 {
				addr := parseStreamSource("audio", b[:6])
				ep.Address = &addr
			}
		case audioEntryParent:
			if pid, ok := f.u8(base + 8); ok {
				p := int(pid)
				ep.parentID = &p
				if parent := eps.Get(p); parent != nil {
					parent.children = append(parent.children, ep)
					ep.parent = parent
				}
			}
		case audioEntryRouteIn:
			if supp, ok := f.u32(base + 8); ok {
				if act, ok := f.u32(base + 12); ok {
					ep.inRoutesSupp = &supp
					ep.inRoutes = &act
				}
			}
		}
	}
	return eps
}

func audioHasLinks(f *frame, nb int) bool {
	return len(f.payload()) > 36+nb*16
}

func parseAudioLinks(f *frame, idx int) []AudioLink {
	nb, ok := f.u16(idx)
	if !ok {
		return nil
	}
	var rv []AudioLink
	for x := 0; x < int(nb); x++ {
		i := 4 + idx + x*20
		ep, ok := f.u8(i)
		if !ok {
			break
		}
		lep, _ := f.u8(i + 1)
		dev, _ := f.uuid(i + 4)
		if dev.Empty() {
			continue
		}
		rv = append(rv, AudioLink{Endpoint: int(ep), LinkEndpoint: int(lep), LinkDevice: dev})
	}
	return rv
}
