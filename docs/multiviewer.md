# Multiviewer

A OneIP Multiviewer is reached through its device:

```go
if d.IsOneIPMultiviewer() {
    mv := d.Multiviewer()
}
```

`Device.Multiviewer()` always returns a controller. Its readers report what the
unit has told us so far, so they return zero values until it has.

## Status

```go
mv.ViewMode()                 // MVViewSingle, MVViewPIP, MVViewFourScreenLarge, ...
mv.VideoSource(screen)        // the source shown on one screen
mv.AudioSource()
mv.AudioVolume(); mv.AudioMuted()
mv.PipSize(); mv.PipPosition(); mv.ScreenAspect()
mv.EdidTemplate(); mv.HDCPMode()
mv.OutputMode(); mv.OutputITCMode()
mv.AutoSwitch(); mv.RemoteControl()
mv.ConnectedSource(input)     // (DeviceUID, bool): the unit mapped to an input
mv.MCUVersion(); mv.ScalerVersion()
```

## Control

```go
mv.SetViewMode(mxremote.MVViewFourScreenLarge)
mv.SetVideoSource(0, source)
mv.SetAudioSource(source)
mv.SetAudioVolume(40, false)
mv.SetPipSize(size); mv.SetPipPosition(pos); mv.SetScreenAspect(aspect)
mv.SetEdidTemplate(tmpl); mv.SetHDCPMode(mode)
mv.SetOutputMode(mode); mv.SetOutputITCMode(mode)
mv.SetAutoSwitch(true)
mv.SetRemoteControl(source)
mv.SetConnectedSource(0, uid)
mv.AutoRoute()
```

Commands sent by other controllers on the network arrive as
`OnMultiviewerCommand`, carrying the raw sub-command and its parameters. The
multiviewer opcode is owned by the multiviewer module rather than by MatrixOS,
so parameters past the envelope are exposed as bytes rather than as named
fields.

## Tiling

`Device.Tiling()` returns the tiling configuration a V2IP sink reported, and
`OnTilingChanged` reports changes to it. This library decodes tiling but does
not transmit it.

## Video wall

`OnVideoWallCommand` reports video wall commands seen on the network, and
`Device.SupportsVideoWall()` says whether a device runs the module that
implements them.

A `VideoWallCommand` carries an `Op`: preview, store or revert. Two properties
matter when reading one:

- `HasWindow()` is false for a revert, which carries no window at all.
- `Cleared()` is true when the window has a zero width or height, which means
  clear the wall rather than leave the setting unset.

A video wall command replaces the wall configuration; unlike the V2IP device
configuration, no field carries a validity marker and nothing is merged.
