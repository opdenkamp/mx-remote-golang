// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

// Library and protocol versions.
const (
	// Version is the version of this library.
	Version = "2.1.3"

	// ProtocolVersion is the highest MX Remote protocol version understood. It
	// is announced in the hello frame and stamped on an opcode with no floor of
	// its own; nothing on the receive path consults it, since a frame is
	// decoded whatever it is stamped.
	//
	// Raising it declares that every layout up to that version is understood,
	// so raise it only alongside the decoding. 0x29 is the decoder detail block
	// of 0x3F V2IP_STATS.
	ProtocolVersion uint16 = 0x29
)

// Network defaults.
const (
	// BroadcastPort is the UDP port used in broadcast mode.
	BroadcastPort = 8811

	// MulticastIP is the multicast group address used for discovery.
	MulticastIP = "224.8.8.8"

	// MulticastPort is the UDP port used in multicast mode.
	MulticastPort = 8812
)

// Default V2IP stream destination UDP ports.
const (
	V2IPPortVideo = 50020
	V2IPPortANC   = 50021
	V2IPPortAudio = 50022
)

// V2IP audio defaults.
const (
	V2IPAudioDefaultSampleRate = 48000
	V2IPAudioDefaultChannels   = 2
	V2IPAudioMinChannels       = 1
	V2IPAudioMaxChannels       = 8
)

// Valid encoder TX rate range, in units of 10Mb/s. A V2IP device-config sender
// with no rate to offer puts a value outside this range in tx_rate; the
// firmware drops that as invalid and keeps the rate it already had, which is
// what stops an address-only or scaling-only write from resetting a peer.
const (
	V2IPSourceRateMin = 5
	V2IPSourceRateMax = 100
)

// Per-stream DSCP marking carried in a V2IP device configuration.
const (
	// V2IPDscpSet is OR'd into a dscp byte alongside its value. DSCP 0 (CS0) is
	// a legal marking, so a zero byte means no marking rather than CS0.
	V2IPDscpSet = 0x80

	// V2IPDscpMax is the highest DSCP value; the marking occupies the upper 6
	// bits of the IPv4 TOS byte.
	V2IPDscpMax = 63

	// V2IPDscpDefault is CS2, the marking the video processor applies at boot
	// and the value firmware falls back to when a peer sends none.
	V2IPDscpDefault = 16
)

// Frame opcodes.
const (
	opSysHello              uint16 = 0x00
	opSysDiscover           uint16 = 0x01
	opSysBayConfig          uint16 = 0x02
	opSysLinks              uint16 = 0x03
	opDevConnect            uint16 = 0x04
	opDevPowerChange        uint16 = 0x05
	opDevSignalOld          uint16 = 0x06
	opDevEDID               uint16 = 0x07
	opMxRoute               uint16 = 0x08
	opMxSetRoute            uint16 = 0x09
	opRCIr                  uint16 = 0x0A
	opRCKey                 uint16 = 0x0B
	opRCTxKey               uint16 = 0x0C
	opRCAction              uint16 = 0x0D
	opRCTxAction            uint16 = 0x0E
	opAudioVolumeUp         uint16 = 0x0F
	opAudioVolumeDown       uint16 = 0x10
	opAudioVolumeMute       uint16 = 0x12
	opAudioClip             uint16 = 0x11
	opAudioSetRoute         uint16 = 0x13
	opAudioSetVolume        uint16 = 0x14
	opSysTemperature        uint16 = 0x15
	opPDUState              uint16 = 0x16
	opV2IPSourceSwitch      uint16 = 0x1F
	opV2IPLinkRemote        uint16 = 0x20
	opV2IPDetectBays        uint16 = 0x21
	opChangeBayName         uint16 = 0x22
	opSysBayConfigSecondary uint16 = 0x23
	opV2IPManualSrcSwitch   uint16 = 0x24
	opSysBayV2IPSources     uint16 = 0x26
	opBayHide               uint16 = 0x27
	opSysReboot             uint16 = 0x28
	opNetLinkStatus         uint16 = 0x29
	opFirmwareVersion       uint16 = 0x2A
	opSysMonitoringPulse    uint16 = 0x2B
	opV2IPUpgradeFPGA       uint16 = 0x2C
	opV2IPBlistRegister     uint16 = 0x2E
	opV2IPBlistUnregister   uint16 = 0x2F
	opTopology              uint16 = 0x30
	opBaySignalStatus       uint16 = 0x31
	opBayMirrorStatus       uint16 = 0x32
	opBayEDIDProfile        uint16 = 0x34
	opSetupStatus           uint16 = 0x35
	opSetMaster             uint16 = 0x36
	opSetInstaller          uint16 = 0x37
	opBayFilterStatus       uint16 = 0x38
	opBayStatus             uint16 = 0x39
	opSysFactoryReset       uint16 = 0x3A
	opMeshOperation         uint16 = 0x3B
	opV2IPDeviceCfg         uint16 = 0x3C
	opAmpZoneSettings       uint16 = 0x3D
	opAmpDolbyState         uint16 = 0x3E
	opV2IPStats             uint16 = 0x3F
	opV2IPTiling            uint16 = 0x40
	opV2IPPowerSave         uint16 = 0x41
	opV2IPMultiviewer       uint16 = 0x42
	opV2IPAudio             uint16 = 0x43
	opV2IPBayMappings       uint16 = 0x44
	opRCSettings            uint16 = 0x45
	opSysStatus             uint16 = 0x46
	opDebug                 uint16 = 0x47
	opRCIrTx                uint16 = 0x48
	opV2IPVideoWall         uint16 = 0x49
)

// opcodeProtocol is the minimum protocol version a receiver needs to decode an
// opcode, mirroring the third argument of MXR_OPCODE() in
// libP8/mx_remote/inc/mx_opcodes.h. A transmitter stamps this in the frame
// header: the receive gate drops any frame whose header protocol exceeds the
// receiver's own MXR_PROTOCOL_VERSION, so stamping ProtocolVersion on
// everything would make a device with a lower cap - a ProAmp8 caps at 0x22 -
// silently drop every frame we send.
//
// These stay deliberately low, and diverge from the firmware's own table where
// they have to. An opcode whose payload only ever grew trailing fields keeps
// its original version, and a receiver tells the formats apart by payload
// length instead: 0x3F V2IP_STATS is 0x29 in the firmware's table since the
// decoder block was appended, and 0x13 here, because the gate is a ceiling
// with no per-opcode minimum. 0x13 is accepted by every version, where a
// 0x29-stamped request is dropped outright by any firmware predating the
// block - costing the counters, not merely the block. Raising it here would also make
// requireOpcodeLocked refuse every device below 0x29 outright.
//
// Opcodes 0x25 and 0x33 are retired and reserved at version 0x00. They were
// live in an earlier generation, so an old unit would decode a frame sent on
// one as whatever it used to be - never reuse them.
var opcodeProtocol = map[uint16]uint16{
	opSysHello:              0x01,
	opSysDiscover:           0x01,
	opSysBayConfig:          0x01,
	opSysLinks:              0x01,
	opDevConnect:            0x1B,
	opDevPowerChange:        0x01,
	opDevSignalOld:          0x01,
	opDevEDID:               0x01,
	opMxRoute:               0x01,
	opMxSetRoute:            0x01,
	opRCIr:                  0x19,
	opRCKey:                 0x01,
	opRCTxKey:               0x0C,
	opRCAction:              0x01,
	opRCTxAction:            0x0C,
	opAudioVolumeUp:         0x01,
	opAudioVolumeDown:       0x01,
	opAudioVolumeMute:       0x01,
	opAudioClip:             0x01,
	opAudioSetRoute:         0x01,
	opAudioSetVolume:        0x11,
	opSysTemperature:        0x01,
	opPDUState:              0x01,
	opV2IPSourceSwitch:      0x06,
	opV2IPLinkRemote:        0x06,
	opV2IPDetectBays:        0x06,
	opChangeBayName:         0x06,
	opSysBayConfigSecondary: 0x07,
	opV2IPManualSrcSwitch:   0x07,
	opSysBayV2IPSources:     0x09,
	opBayHide:               0x06,
	opSysReboot:             0x01,
	opNetLinkStatus:         0x22,
	opFirmwareVersion:       0x06,
	opSysMonitoringPulse:    0x01,
	opV2IPUpgradeFPGA:       0x06,
	opV2IPBlistRegister:     0x06,
	opV2IPBlistUnregister:   0x06,
	opTopology:              0x06,
	opBaySignalStatus:       0x06,
	opBayMirrorStatus:       0x06,
	opBayEDIDProfile:        0x08,
	opSetupStatus:           0x0A,
	opSetMaster:             0x0B,
	opSetInstaller:          0x0C,
	opBayFilterStatus:       0x0E,
	opBayStatus:             0x0F,
	opSysFactoryReset:       0x0F,
	opMeshOperation:         0x1D,
	opV2IPDeviceCfg:         0x11,
	opAmpZoneSettings:       0x1C,
	opAmpDolbyState:         0x1C,
	opV2IPStats:             0x13,
	opV2IPTiling:            0x14,
	opV2IPPowerSave:         0x15,
	opV2IPMultiviewer:       0x16,
	opV2IPAudio:             0x1A,
	opV2IPBayMappings:       0x1C,
	opRCSettings:            0x1D,
	opSysStatus:             0x1E,
	opDebug:                 0x1F,
	opRCIrTx:                0x23,
	opV2IPVideoWall:         0x28,
}

// protocolFor returns the header protocol version to stamp on a frame carrying
// opcode.
func protocolFor(opcode uint16) uint16 {
	if v, ok := opcodeProtocol[opcode]; ok {
		return v
	}
	return ProtocolVersion
}
