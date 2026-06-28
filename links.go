// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

// BayLink is a virtual link between two bays on two devices (e.g. an amp output
// linked to a OneIP sink). All exported methods are safe for concurrent use.
type BayLink struct {
	r            *Remote
	bay          *Bay
	linkedSerial string
	linkedBay    string
	features     uint32
}

// Serial returns the serial of the linked device.
func (l *BayLink) Serial() string { return l.linkedSerial }

// LinkedBayName returns the name of the linked bay.
func (l *BayLink) LinkedBayName() string { return l.linkedBay }

// Configured reports whether a link has been set up.
func (l *BayLink) Configured() bool {
	l.r.mu.Lock()
	defer l.r.mu.Unlock()
	return l.linkedLocked()
}

// LinkedBay returns the bay on the other end of this link, or nil.
func (l *BayLink) LinkedBay() *Bay {
	l.r.mu.Lock()
	defer l.r.mu.Unlock()
	return l.linkedBayLocked()
}

// IsAudio reports whether this link carries audio.
func (l *BayLink) IsAudio() bool {
	l.r.mu.Lock()
	defer l.r.mu.Unlock()
	m := l.featuresMaskLocked()
	return m.Has(LinkAudioOptical) || m.Has(LinkAudioAnalog)
}

// IsVideo reports whether this link carries HDMI video.
func (l *BayLink) IsVideo() bool {
	l.r.mu.Lock()
	defer l.r.mu.Unlock()
	return l.featuresMaskLocked().Has(LinkVideoHDMI)
}

func (l *BayLink) linkedLocked() bool {
	return l.linkedSerial != "" && l.linkedBay != ""
}

func (l *BayLink) linkedBayLocked() *Bay {
	if !l.linkedLocked() {
		return nil
	}
	return l.r.getBayByPortnameLocked(l.linkedSerial, l.linkedBay)
}

func (l *BayLink) otherLinkLocked() *BayLink {
	lb := l.linkedBayLocked()
	if lb == nil {
		return nil
	}
	return l.r.links.getLocked(lb)
}

func (l *BayLink) connectedLocked() bool {
	ol := l.otherLinkLocked()
	return ol != nil && ol.linkedLocked() &&
		ol.linkedSerial == l.bay.dev.serialLocked() && ol.linkedBay == l.bay.portName
}

func (l *BayLink) featuresMaskLocked() LinkFeature {
	if !l.connectedLocked() {
		return LinkNone
	}
	lb := l.linkedBayLocked()
	if lb == nil {
		return LinkNone
	}
	left, right := l.bay.features, lb.features
	var rv LinkFeature
	pair := func(a, b BayFeaturesMask, f LinkFeature) {
		if (left.Has(a) && right.Has(b)) || (left.Has(b) && right.Has(a)) {
			rv |= f
		}
	}
	pair(BayHDMIOut, BayHDMIIn, LinkVideoHDMI)
	pair(BayAudioDigOut, BayAudioDigIn, LinkAudioOptical)
	pair(BayAudioAnaOut, BayAudioAnaIn, LinkAudioAnalog)
	pair(BayIROut, BayIRIn, LinkIR)
	pair(BayRCOut, BayRCIn, LinkRC)
	return rv
}

// bayLinks holds the virtual link configuration for all devices, keyed by the
// origin bay's cross-device UID.
type bayLinks struct {
	r     *Remote
	links map[BayUID]*BayLink
}

func newBayLinks(r *Remote) *bayLinks {
	return &bayLinks{r: r, links: map[BayUID]*BayLink{}}
}

func (bl *bayLinks) getLocked(b *Bay) *BayLink {
	if b == nil {
		return nil
	}
	return bl.links[b.bayUIDLocked()]
}

func (bl *bayLinks) update(b *Bay, linkedSerial, linkedBay string, features uint32) {
	nl := &BayLink{r: bl.r, bay: b, linkedSerial: linkedSerial, linkedBay: linkedBay, features: features}
	key := b.bayUIDLocked()
	old, ok := bl.links[key]
	if ok {
		if old.linkedSerial == nl.linkedSerial && old.features == nl.features {
			return
		}
		if old.linkedLocked() {
			bl.fireUnlinked(b, old.linkedSerial, b.portName)
			if ob := old.linkedBayLocked(); ob != nil {
				bl.fireUnlinked(ob, b.dev.serialLocked(), b.portName)
			}
		}
	}
	bl.links[key] = nl
	if nl.linkedLocked() {
		fm := nl.featuresMaskLocked()
		bl.fireLinked(b, nl.linkedSerial, b.portName, fm)
		if ob := nl.linkedBayLocked(); ob != nil {
			bl.fireLinked(ob, b.dev.serialLocked(), b.portName, fm)
		}
	}
}

func (bl *bayLinks) fireLinked(b *Bay, serial, bayName string, fm LinkFeature) {
	r := bl.r
	if r.callbacks.OnBayLinked != nil {
		r.emit(func() { r.callbacks.OnBayLinked(b, serial, bayName, fm) })
	}
	bl.fanBayDevice(b)
}

func (bl *bayLinks) fireUnlinked(b *Bay, serial, bayName string) {
	r := bl.r
	if r.callbacks.OnBayUnlinked != nil {
		r.emit(func() { r.callbacks.OnBayUnlinked(b, serial, bayName) })
	}
	bl.fanBayDevice(b)
}

// fanBayDevice mirrors the reference on_bay_linked/on_bay_unlinked fan-in to
// both on_device_update and on_bay_update.
func (bl *bayLinks) fanBayDevice(b *Bay) {
	r := bl.r
	if r.callbacks.OnDeviceUpdate != nil {
		r.emit(func() { r.callbacks.OnDeviceUpdate(b.dev) })
	}
	if r.callbacks.OnBayUpdate != nil {
		r.emit(func() { r.callbacks.OnBayUpdate(b) })
	}
	b.emitSelf()
	b.dev.emitSelf()
}
