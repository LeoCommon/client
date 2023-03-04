package iridium

import (
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client/errors"
)

const (
	HACKRF_CONFIG_TEMPLATE = `[osmosdr-source]
sample_rate=%d
center_freq=%d
bandwidth=%d
gain=%d
if_gain=%d
bb_gain=%d
`
	// The time after which the startup check should be considered timed out
	STARTUP_CHECK_TIMEOUT = 10 * time.Second
)

var (
	STARTUP_CHECK_STRINGS = []StartupResult{
		// Return if we found using hackrf one
		{"using hackrf one", nil},
		// Indicates the usb is busy and the sdr stuck
		{"resource busy", &errors.SDRStuckError{}},
		// No SDR attached
		{"no supported devices found", &errors.SDRNotFoundError{}},
	}
)
