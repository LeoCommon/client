package sim7600

import (
	"bufio"
	"errors"
	"log"
	"strings"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/modem"
	"disco.cs.uni-kl.de/apogee/pkg/modem/sim7600/atparser"
	"github.com/tarm/serial"
	"go.uber.org/zap"
)

var (
	GPS_TTY       = "/dev/serial/by-id/usb-SimTech__Incorporated_SimTech__Incorporated_0123456789ABCDEF-if01-port0"
	MGMT_TTY      = "/dev/serial/by-id/usb-SimTech__Incorporated_SimTech__Incorporated_0123456789ABCDEF-if02-port0"
	MGMT_BAUDRATE = 115200

	AT_GPS_STATE       = "AT+CGPS"
	AT_GPS_STATE_QUERY = AT_GPS_STATE + "?"

	AT_RESET_MODEM = "AT+CRESET"

	AT_GPS_AUTOSTART = "AT+CGPSAUTO=" // add ? or 1

	AT_REPLY_OK    = "OK"
	AT_REPLY_ERROR = "ERROR"
)

type SIM7600Modem struct {
	modem.Modem

	serConf *serial.Config
	serPort *serial.Port

	gpsMode    atparser.GPSModeEnum
	gpsStarted bool
}

func Create(customMGMTSerialConfig *serial.Config) *SIM7600Modem {
	m := new(SIM7600Modem)

	// Use provided serial config if adjusted
	if customMGMTSerialConfig != nil {
		m.serConf = customMGMTSerialConfig
	} else {
		m.serConf = &serial.Config{
			Name:        MGMT_TTY,
			Baud:        MGMT_BAUDRATE,
			ReadTimeout: time.Second * 1}
	}

	m.gpsMode = atparser.GPS_MODE_STANDALONE
	return m
}

func (m *SIM7600Modem) readSerial(n int) (string, error) {
	buf := make([]byte, n)
	_, readRes := m.serPort.Read(buf)
	if readRes != nil {
		apglog.Error("serial read failed", zap.Int("n", n))
		return "", readRes
	}

	return modem.TrimCRLF(modem.ByteSliceToStr(buf)), nil
}

func (m *SIM7600Modem) writeSerial(data string) error {
	_, err := m.serPort.Write([]byte(string(data) + "\r\n"))
	return err
}

func (m *SIM7600Modem) writeSerialWithResult(data string, expectedResult string) error {
	apglog.Debug("Writing serial data with result", zap.String("data", data))
	writeRes := m.writeSerial(data)
	if writeRes != nil {
		return writeRes
	}

	// Read 128 bytes, the result reader is for basic stuff like ERROR or OK
	strBuf, readRes := m.readSerial(128)
	if readRes != nil {
		return errors.New("serial reading failed")
	}

	if expectedResult != strBuf {
		apglog.Error("serial write with result failed", zap.String("expected", expectedResult), zap.String("received", strBuf))
		return errors.New("serial write with result failed")
	}

	return nil
}

func (m *SIM7600Modem) Open() error {
	s, err := serial.OpenPort(m.serConf)
	if err != nil || s == nil {
		log.Fatal(err)
		return err
	}

	// Assign serial port
	m.serPort = s

	// Try to retrieve the gps mode
	m.determineGPSMode()
	return nil
}

func (m *SIM7600Modem) initialized() error {
	if m.serPort == nil {
		return errors.New("serial port not ready")
	}

	return nil
}

func (m *SIM7600Modem) Reset() error {
	initError := m.initialized()
	if initError != nil {
		return initError
	}

	// Send reset command
	return m.writeSerialWithResult(AT_RESET_MODEM, AT_REPLY_OK)
}

/*
	Determines the state of the gps device and retrieves the mode correctly
*/
func (m *SIM7600Modem) determineGPSMode() error {
	err := m.writeSerial(AT_GPS_STATE_QUERY)
	if err != nil {
		return err
	}

	data, err := m.readSerial(128)
	if err != nil {
		return err
	}

	// Match the Response
	if data == AT_REPLY_ERROR {
		m.gpsMode = atparser.GPS_MODE_UNKNOWN
		return errors.New("error received while trying to query gps status")
	}

	scanner := bufio.NewScanner(strings.NewReader(data))

	scanner.Scan()
	modeResult := scanner.Bytes()

	// Skip empty line (\r\n)
	scanner.Scan()

	// Get result code
	scanner.Scan()
	resultCode := scanner.Bytes()
	if modem.MSTR(resultCode) != AT_REPLY_OK {
		m.gpsMode = atparser.GPS_MODE_UNKNOWN
		return errors.New("modem did not reply with AT OK got: " + string(modeResult))
	}

	// Get the actual mode from the device
	started, mode, err := atparser.GPSStatus(modem.MSTR(modeResult))

	if err != nil {
		return err
	}

	m.gpsStarted = started
	m.gpsMode = mode
	return nil
}

type GPSNotYetReadyError struct{}

func (e *GPSNotYetReadyError) Error() string {
	return "gps not ready, wait up to 30 seconds and try again"
}

/* Try to start the GPS, only standalone mode is supported */
func (m *SIM7600Modem) StartGPS(desiredMode atparser.GPSModeEnum) error {
	if desiredMode != atparser.GPS_MODE_STANDALONE {
		return errors.New("unsupported GPS mode, only standalone supported for now")
	}

	initError := m.initialized()
	if initError != nil {
		return initError
	}

	// Retrieve the current gps mode
	err := (m.determineGPSMode())
	if err != nil {
		return err
	}

	// If the gps is not offline but in a mode thats not standalone, reset
	gpsStopInitiated := false
	if m.gpsMode != atparser.GPS_MODE_OFFLINE && m.gpsMode != desiredMode {
		// Reset the gps
		err = m.StopGPS()
		if err != nil {
			apglog.Error("Failed to stop gps", zap.String("error", err.Error()))
			return err
		}
		gpsStopInitiated = true

		// #todo improvement: the device prints +CGPS: 0 after some time,
		// so we could theoretically poll here and block until the reset commences
	}

	// Dont start gps because we are already up and running
	if m.gpsStarted && m.gpsMode == desiredMode {
		apglog.Debug("GPS already started", zap.String("mode", string(m.gpsMode)))
		return nil
	}

	// Send GPS Start command
	err = m.writeSerialWithResult(AT_GPS_STATE+"=1,"+string(desiredMode), AT_REPLY_OK)
	if err != nil {
		// If we recently stopped the GPS, this error is to be expected
		if gpsStopInitiated {
			return &GPSNotYetReadyError{}
		} else {
			return err
		}
	}

	// Re-run mode check to verify that the modem started in the mode we wanted
	m.determineGPSMode()

	if desiredMode != m.gpsMode {
		apglog.Error("GPS mode mismatch while starting", zap.String("wanted", string(desiredMode)), zap.String("got", string(m.gpsMode)))
	}

	apglog.Info("GPS started", zap.String("mode", string(m.gpsMode)))
	return nil
}

func (m SIM7600Modem) StopGPS() error {
	initError := m.initialized()
	if initError != nil {
		return initError
	}

	return m.writeSerialWithResult(AT_GPS_STATE+"=0", AT_REPLY_OK)
}

func (m SIM7600Modem) GetGPSState() (bool, error) {
	initError := m.initialized()
	if initError != nil {
		return false, initError
	}

	return true, nil
}

func (m *SIM7600Modem) ResetGPS() error {
	initError := m.initialized()
	if initError != nil {
		return initError
	}

	return nil
}

/**
The following things are stubs not needed for the device
**/
func (m *SIM7600Modem) Enable() error {
	return nil
}

func (m *SIM7600Modem) Disable() error {
	return nil
}
