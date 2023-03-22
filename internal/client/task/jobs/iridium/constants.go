package iridium

import (
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/usb"
)

const (
	HackrfConfigTemplate = `[osmosdr-source]
sample_rate=%d
center_freq=%d
bandwidth=%d
gain=%d
if_gain=%d
bb_gain=%d

# demodulator settings for pi4
[demodulator]
decimation=16
samples_per_symbol=5
`
	// StartupCheckTimeout The time after which the startup check should be considered timed out
	StartupCheckTimeout = 10 * time.Second
)

var (
	StartupCheckStrings = []StartupResult{
		// Return if we found using hackrf one
		{Str: "using hackrf one", Err: nil},
		// Indicates the usb is busy and the sdr stuck
		{Str: "resource busy", Err: usb.NewStuckError("device stuck with resource busy")},
		// No SDR attached
		{Str: "no supported devices found", Err: usb.NewNotFoundError("no supported devices")},
	}
)
