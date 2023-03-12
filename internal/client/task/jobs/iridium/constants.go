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
`
	// StartupCheckTimeout The time after which the startup check should be considered timed out
	StartupCheckTimeout = 10 * time.Second
)

var (
	StartupCheckStrings = []StartupResult{
		// Return if we found using hackrf one
		{"using hackrf one", nil},
		// Indicates the usb is busy and the sdr stuck
		{"resource busy", usb.NewStuckError("device stuck with resource busy")},
		// No SDR attached
		{"no supported devices found", usb.NewNotFoundError("no supported devices")},
	}
)
