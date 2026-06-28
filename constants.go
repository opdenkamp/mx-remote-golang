// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

// Library and protocol versions.
const (
	// Version is the version of this library.
	Version = "1.1.0"

	// ProtocolVersion is the highest MX Remote protocol version understood.
	ProtocolVersion uint16 = 0x27
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
	opAudioSetVolume        uint16 = 0x14
	opSysTemperature        uint16 = 0x15
	opPDUState              uint16 = 0x16
	opV2IPSourceSwitch      uint16 = 0x1F
	opV2IPLinkRemote        uint16 = 0x20
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
)
