package atparser

import (
	"fmt"
)

type GPSModeEnum string

const (
	GpsModeUnknown    GPSModeEnum = "-1"
	GpsModeOffline    GPSModeEnum = "0"
	GpsModeStandalone GPSModeEnum = "1"
	GpsModeUe         GPSModeEnum = "2"
	GpsModeAssisted   GPSModeEnum = "3"
)

// GPSStatus Parses +CGPS: ?,? into the gps status
func GPSStatus(line string) (started bool, mode GPSModeEnum, err error) {
	if len(line) < 10 {
		err = fmt.Errorf("cant parse, input too short %v", line)
		return
	}

	header := line[0:6]
	if header != "+CGPS:" {
		err = fmt.Errorf("unknown header %v", line)
		return
	}

	startStr := line[7]
	if startStr == '1' {
		started = true
	} else if startStr != '0' {
		err = fmt.Errorf("unknown gps status %v", line)
		return
	}

	gpsModeStr := line[9]
	switch gpsModeStr {
	case '0':
		mode = GpsModeOffline
	case '1':
		mode = GpsModeStandalone
	case '2':
		mode = GpsModeUe
	case '3':
		mode = GpsModeAssisted
	default:
		mode = GpsModeUnknown
		err = fmt.Errorf("unknown gps mode %v", line)
	}

	return
}
