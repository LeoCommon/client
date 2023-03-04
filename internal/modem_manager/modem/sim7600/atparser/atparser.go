package atparser

import (
	"fmt"
)

type GPSModeEnum string

const (
	GPS_MODE_UNKNOWN    GPSModeEnum = "-1"
	GPS_MODE_OFFLINE    GPSModeEnum = "0"
	GPS_MODE_STANDALONE GPSModeEnum = "1"
	GPS_MODE_UE         GPSModeEnum = "2"
	GPS_MODE_ASSISTED   GPSModeEnum = "3"
)

// Parses +CGPS: ?,? into the gps status
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
		mode = GPS_MODE_OFFLINE
	case '1':
		mode = GPS_MODE_STANDALONE
	case '2':
		mode = GPS_MODE_UE
	case '3':
		mode = GPS_MODE_ASSISTED
	default:
		mode = GPS_MODE_UNKNOWN
		err = fmt.Errorf("unknown gps mode %v", line)
	}

	return
}
