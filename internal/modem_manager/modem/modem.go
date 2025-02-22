package modem

import (
	"bytes"
	"strings"

	"github.com/LeoCommon/client/internal/modem_manager/modem/sim7600/atparser"
)

type Modem interface {
	// Open the modem management connection
	Open() error
	// Close Tear down the modem and optionally reset it
	Close(reset bool) error

	// Enable the modem if needed
	Enable() error
	// Disable the modem if possible
	Disable() error
	// Reset the modem if supported
	Reset() error

	// StartGPS GPS
	StartGPS(desiredMode atparser.GPSModeEnum, forceRestart bool) error
	StopGPS() error
	ResetGPS() error

	GetGPSState() (bool, error)
}

// ByteSliceToStr converts a byte slice to string
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
