package modem

import (
	"bytes"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/modem/sim7600/atparser"
)

type Modem interface {
	// Open the modem management connection
	Open() error
	// Tear down the modem and optionally reset it
	Close(reset bool) error

	// Enable the modem if needed
	Enable() error
	// Disable the modem if possible
	Disable() error
	// Reset the modem if supported
	Reset() error

	// GPS
	StartGPS(desiredMode atparser.GPSModeEnum, forceRestart bool) error
	StopGPS() error
	ResetGPS() error

	GetGPSState() (bool, error)
}

// Some generic helper functions
func ByteSliceToStr(s []byte) string {
	n := bytes.IndexByte(s, 0)
	if n >= 0 {
		s = s[:n]
	}
	return string(s)
}

func TrimCRLF(s string) string {
	return strings.Trim(s, "\r\n")
}

func MSTR(b []byte) string {
	return TrimCRLF(ByteSliceToStr(b))
}
