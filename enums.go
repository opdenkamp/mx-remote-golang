// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import "fmt"

// DeviceFeature is the bitmask of capabilities a device reports in its hello frame.
type DeviceFeature uint32

const (
	FeatureIRRx           DeviceFeature = 1 << 0
	FeatureIRTx           DeviceFeature = 1 << 1
	FeatureCEC            DeviceFeature = 1 << 2
	FeatureV2IPSource     DeviceFeature = 1 << 3
	FeatureV2IPSink       DeviceFeature = 1 << 4
	FeatureVideoRouting   DeviceFeature = 1 << 5
	FeatureAudioRouting   DeviceFeature = 1 << 6
	FeatureVolumeControl  DeviceFeature = 1 << 7
	FeatureAudioReturn    DeviceFeature = 1 << 8
	FeatureRemoteControl  DeviceFeature = 1 << 9
	FeatureSetupCompleted DeviceFeature = 1 << 10
	FeatureMeshMaster     DeviceFeature = 1 << 11
	FeatureStatusNotify   DeviceFeature = 1 << 12
	FeatureStatusWarning  DeviceFeature = 1 << 13
	FeatureStatusError    DeviceFeature = 1 << 14
	FeatureStatusReboot   DeviceFeature = 1 << 15
	FeatureMeshMember     DeviceFeature = 1 << 16
	FeatureAudioAmplifier DeviceFeature = 1 << 17
	FeatureBooting        DeviceFeature = 1 << 18
	FeatureManager        DeviceFeature = 1 << 19
	FeatureStatusPowSave  DeviceFeature = 1 << 20
	FeatureMesh           DeviceFeature = 1 << 21
	FeatureMultiviewer    DeviceFeature = 1 << 22
	FeatureStatusCrashed  DeviceFeature = 1 << 23
	FeatureBootBit        DeviceFeature = 1 << 31
)

// Has reports whether all bits in f are set.
func (d DeviceFeature) Has(f DeviceFeature) bool { return d&f == f }

// BayFeaturesMask is the bitmask of a bay's capabilities.
type BayFeaturesMask uint32

const (
	BayHDMIOut          BayFeaturesMask = 1 << 0
	BayHDMIIn           BayFeaturesMask = 1 << 1
	BayAudioDigOut      BayFeaturesMask = 1 << 2
	BayAudioDigIn       BayFeaturesMask = 1 << 3
	BayAudioAnaOut      BayFeaturesMask = 1 << 4
	BayAudioAnaIn       BayFeaturesMask = 1 << 5
	BayIRIn             BayFeaturesMask = 1 << 6
	BayIROut            BayFeaturesMask = 1 << 7
	BayAudioAmpOut      BayFeaturesMask = 1 << 8
	BayRCOut            BayFeaturesMask = 1 << 9
	BayRCIn             BayFeaturesMask = 1 << 10
	BayDolby            BayFeaturesMask = 1 << 11
	BayAutoOff          BayFeaturesMask = 1 << 12
	BayV2IPSourceRemote BayFeaturesMask = 1 << 13
	BayV2IPSinkRemote   BayFeaturesMask = 1 << 14
	BayV2IPSourceLocal  BayFeaturesMask = 1 << 15
	BayV2IPSinkLocal    BayFeaturesMask = 1 << 16
)

func (b BayFeaturesMask) Has(f BayFeaturesMask) bool { return b&f == f }

const bayFeatureDolbyInPos = 24

// BayStatusMask is the bitmask of a bay's live status flags. Note that bits
// 16-19 (RC type) and 22-23 (HDCP) are bit-fields, not flags; use
// BayStatusRCType and BayStatusHDCP to extract them.
type BayStatusMask uint32

const (
	BayStatusFault          BayStatusMask = 1 << 0
	BayStatusHidden         BayStatusMask = 1 << 1
	BayStatusPowered        BayStatusMask = 1 << 2
	BayStatusSignalDetected BayStatusMask = 1 << 3
	BayStatusHPDDetected    BayStatusMask = 1 << 4
	BayStatusSignalScramble BayStatusMask = 1 << 5
	BayStatusHDBTConnected  BayStatusMask = 1 << 6
	BayStatusCECDetected    BayStatusMask = 1 << 7
	BayStatusPoweredOn      BayStatusMask = 1 << 8
	BayStatusPoweredOff     BayStatusMask = 1 << 9
	BayStatusAudioARCHDMI   BayStatusMask = 1 << 10
	BayStatusAudioARCOptic  BayStatusMask = 1 << 11
	BayStatusAudioARCAnalog BayStatusMask = 1 << 12
	BayStatusOffline        BayStatusMask = 1 << 13
	BayStatusDecoderDisable BayStatusMask = 1 << 14
	BayStatusEncoderDisable BayStatusMask = 1 << 15
	BayStatusCECDisabled    BayStatusMask = 1 << 20
	BayStatusEncoderError   BayStatusMask = 1 << 21
)

func (b BayStatusMask) Has(f BayStatusMask) bool { return b&f == f }

const (
	bayStatusRCTypeShift = 16
	bayStatusRCTypeMask  = 0xF << bayStatusRCTypeShift
	bayStatusHDCPShift   = 22
	bayStatusHDCPMask    = 0x3 << bayStatusHDCPShift
)

// BayStatusRCType extracts the RC type (4 bits) from a bay-status word.
func BayStatusRCType(status uint32) int {
	return int((status & bayStatusRCTypeMask) >> bayStatusRCTypeShift)
}

// BayStatusHDCP extracts the HDCP status (2 bits) from a bay-status word.
func BayStatusHDCP(status uint32) int {
	return int((status & bayStatusHDCPMask) >> bayStatusHDCPShift)
}

// LinkFeature is the bitmask of media carried by a virtual link.
type LinkFeature uint32

const (
	LinkNone         LinkFeature = 0
	LinkVideoHDMI    LinkFeature = 1 << 0
	LinkAudioOptical LinkFeature = 1 << 1
	LinkAudioAnalog  LinkFeature = 1 << 2
	LinkIR           LinkFeature = 1 << 3
	LinkRC           LinkFeature = 1 << 4
)

func (l LinkFeature) Has(f LinkFeature) bool { return l&f == f }

// RCAction is a remote-control action.
type RCAction int

const (
	ActionPowerToggle RCAction = 0
	ActionPowerOn     RCAction = 1
	ActionPowerOff    RCAction = 2
	ActionVolumeDown  RCAction = 3
	ActionVolumeUp    RCAction = 4
	ActionVolumeMute  RCAction = 5
)

// RCKey is a remote-control key code (CEC/IR).
type RCKey int

const (
	KeyNum0          RCKey = 0
	KeyNum1          RCKey = 1
	KeyNum2          RCKey = 2
	KeyNum3          RCKey = 3
	KeyNum4          RCKey = 4
	KeyNum5          RCKey = 5
	KeyNum6          RCKey = 6
	KeyNum7          RCKey = 7
	KeyNum8          RCKey = 8
	KeyNum9          RCKey = 9
	KeySelect        RCKey = 10
	KeyBack          RCKey = 11
	KeyUp            RCKey = 12
	KeyDown          RCKey = 13
	KeyLeft          RCKey = 14
	KeyRight         RCKey = 15
	KeyMenu          RCKey = 16
	KeyContentMenu   RCKey = 17
	KeyChannelUp     RCKey = 18
	KeyChannelDown   RCKey = 19
	KeyPlay          RCKey = 20
	KeyPause         RCKey = 21
	KeyStop          RCKey = 22
	KeyRecord        RCKey = 23
	KeyFastForward   RCKey = 24
	KeyRewind        RCKey = 25
	KeyRed           RCKey = 26
	KeyGreen         RCKey = 27
	KeyYellow        RCKey = 28
	KeyBlue          RCKey = 29
	KeyHelp          RCKey = 30
	KeyInformation   RCKey = 31
	KeyText          RCKey = 32
	KeyGuide         RCKey = 33
	KeyVideoOnDemand RCKey = 34
	KeyPreviousChan  RCKey = 80
	Key3DMode        RCKey = 81
	KeySubtitle      RCKey = 82
	KeySoundSelect   RCKey = 83
	KeyInputSelect   RCKey = 84
	KeyEject         RCKey = 85
	KeyNextChapter   RCKey = 86
	KeyPrevChapter   RCKey = 87
	KeyInteractive   RCKey = 128
	KeySearch        RCKey = 129
	KeySky           RCKey = 130
	KeyCustomCEC     RCKey = 1280
	KeyCustomSky     RCKey = 2048
)

// RCType is the remote-control protocol of a connected sink/source.
type RCType int

const (
	RCTypeIR       RCType = 0
	RCTypeCEC      RCType = 1
	RCTypeSkyUK    RCType = 2
	RCTypeTiVo     RCType = 3
	RCTypeKodi     RCType = 4
	RCTypeDish     RCType = 5
	RCTypeDirecTV  RCType = 6
	RCTypeMXRemote RCType = 7
)

func (r RCType) String() string {
	switch r {
	case RCTypeIR:
		return "IR"
	case RCTypeCEC:
		return "CEC"
	case RCTypeSkyUK:
		return "Sky"
	case RCTypeTiVo:
		return "TiVo"
	case RCTypeKodi:
		return "Kodi"
	case RCTypeDish:
		return "Dish"
	case RCTypeDirecTV:
		return "DirecTV"
	case RCTypeMXRemote:
		return "MX-Remote"
	}
	return "Unknown"
}

// EdidProfile is an EDID preset selectable on an HDMI input.
type EdidProfile int

const (
	Edid1080PStereo     EdidProfile = 0
	EdidFixed           EdidProfile = 1
	Edid4K              EdidProfile = 2
	Edid1080P51         EdidProfile = 3
	Edid720P            EdidProfile = 4
	Edid1080P71         EdidProfile = 5
	Edid4K71            EdidProfile = 6
	Edid4KHDRStereo     EdidProfile = 7
	Edid4KHDR71         EdidProfile = 8
	Edid4KHDRAVROnly    EdidProfile = 9
	EdidLowestCommon    EdidProfile = 10
	EdidLowestCommonAll EdidProfile = 11
	Edid4KHDRAtmos      EdidProfile = 12
	EdidSink1           EdidProfile = 101
	EdidSink32          EdidProfile = 132
	EdidCustom0         EdidProfile = 500
	EdidUnknown         EdidProfile = 0xFFF
)

func (e EdidProfile) String() string {
	switch e {
	case Edid1080PStereo:
		return "1080p stereo"
	case EdidFixed:
		return "fixed"
	case Edid4K:
		return "4K"
	case Edid1080P51:
		return "1080p 5.1"
	case Edid720P:
		return "720p"
	case Edid1080P71:
		return "1080p 7.1"
	case Edid4KHDRStereo:
		return "4K HDR Stereo"
	case Edid4KHDR71:
		return "4K HDR 7.1"
	case Edid4KHDRAVROnly:
		return "4K HDR AVR"
	case EdidLowestCommon:
		return "lowest common denominator"
	case EdidLowestCommonAll:
		return "lowest common denominator (all sinks)"
	case Edid4KHDRAtmos:
		return "4K HDR Dolby Atmos"
	}
	if e >= EdidSink1 && e <= EdidSink32 {
		return fmt.Sprintf("copy from sink #%d", int(e-EdidSink1)+1)
	}
	return fmt.Sprintf("custom #%d", int(e))
}

// FirmwareType identifies a firmware component.
type FirmwareType int

const (
	FirmwareUnknown        FirmwareType = 0
	FirmwareFPGA           FirmwareType = 1
	FirmwareLinux          FirmwareType = 2
	FirmwareLoadingOverlay FirmwareType = 3
)

func (f FirmwareType) String() string {
	switch f {
	case FirmwareFPGA:
		return "FPGA"
	case FirmwareLinux:
		return "Linux"
	case FirmwareLoadingOverlay:
		return "Loading Overlay"
	}
	return "Unknown"
}

// UtpLinkSpeed is the negotiated speed of a network port.
type UtpLinkSpeed int

const (
	LinkSpeedUnknown UtpLinkSpeed = 0
	LinkSpeed10M     UtpLinkSpeed = 1
	LinkSpeed100M    UtpLinkSpeed = 2
	LinkSpeed200M    UtpLinkSpeed = 3
	LinkSpeed1G      UtpLinkSpeed = 4
)

func (u UtpLinkSpeed) String() string {
	switch u {
	case LinkSpeed10M:
		return "10Mbit/s"
	case LinkSpeed100M:
		return "100Mbit/s"
	case LinkSpeed200M:
		return "200Mbit/s"
	case LinkSpeed1G:
		return "1Gbit/s"
	}
	return "Unknown"
}
