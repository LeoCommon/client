package sim7600

import (
	"bufio"
	"errors"
	"fmt"
	"time"

	"github.com/LeoCommon/client/internal/modem_manager/modem"
	"github.com/LeoCommon/client/internal/modem_manager/modem/sim7600/atparser"
	"github.com/LeoCommon/client/pkg/log"
	"go.bug.st/serial"
	"go.uber.org/zap"
)

const (
	MgmtTty      = "/dev/serial/by-id/usb-SimTech__Incorporated_SimTech__Incorporated_0123456789ABCDEF-if02-port0"
	MgmtBaudrate = 115200

	AtGpsState      = "AT+CGPS"
	AtGpsStateQuery = AtGpsState + "?"

	AtResetModem = "AT+CRESET"

	AtReplyOk    = "OK"
	AtReplyError = "ERROR"
)

type Modem struct {
	modem.Modem

	serConf *serial.Mode
	serPort serial.Port

	gpsMode    atparser.GPSModeEnum
	gpsStarted bool
}

func Create(customMGMTSerialConfig *serial.Mode) *Modem {
	m := new(Modem)

	// Use provided serial config if adjusted
	if customMGMTSerialConfig != nil {
		m.serConf = customMGMTSerialConfig
	} else {
		m.serConf = &serial.Mode{
			BaudRate: MgmtBaudrate,
		}
	}

	m.gpsMode = atparser.GpsModeStandalone
	return m
}

// readSerial
func (m *Modem) readSerial(n int) (string, error) {
	buf := make([]byte, n)
	_, readRes := m.serPort.Read(buf)
	if readRes != nil {
		log.Error("serial read failed", zap.Int("n", n))
		return "", readRes
	}

	return modem.TrimCRLF(modem.ByteSliceToStr(buf)), nil
}

// writeSerial
func (m *Modem) writeSerial(data string) error {
	_, err := m.serPort.Write([]byte(string(data) + "\r\n"))
	return err
}

// writeSerialWithResult
func (m *Modem) writeSerialWithResult(data string, expectedResult string) error {
	log.Debug("Writing serial data with result", zap.String("data", data))
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
		log.Error("serial write with result failed", zap.String("expected", expectedResult), zap.String("received", strBuf))
		return errors.New("serial write with result failed")
	}

	return nil
}

func (m *Modem) Open() error {
	s, err := serial.Open(MgmtTty, m.serConf)
	if err != nil {
		log.Error("error while opening serial device", zap.Error(err))
		return err
	}

	// Set read timeout
	_ = s.SetReadTimeout(1 * time.Second)

	// Assign serial port
	m.serPort = s

	// Try to retrieve the gps mode
	return m.determineGPSMode()
}

func (m *Modem) initialized() error {
	if m.serPort == nil {
		return errors.New("serial port not ready")
	}

	return nil
}

func (m *Modem) Reset() error {
	initError := m.initialized()
	if initError != nil {
		return initError
	}

	// Send reset command
	return m.writeSerialWithResult(AtResetModem, AtReplyOk)
}

/*
Determines the state of the gps device and retrieves the mode correctly
*/
func (m *Modem) determineGPSMode() error {
	err := m.writeSerial(AtGpsStateQuery)
	if err != nil {
		return err
	}

	var read bool

	scanner := bufio.NewScanner(m.serPort)
	read = scanner.Scan()
	if !read {
		return fmt.Errorf("AT#reply - unexpected end of input")
	}

	// Match the Response
	if scanner.Text() == AtReplyError {
		m.gpsMode = atparser.GpsModeUnknown
		return fmt.Errorf("error received while trying to query gps status")
	}

	// Get the actual GPS Mode
	read = scanner.Scan()
	if !read {
		return fmt.Errorf("gpsMode - unexpected end of input")
	}

	modeResult := scanner.Text()

	// Skip empty line
	read = scanner.Scan()
	if !read || len(scanner.Bytes()) != 0 {
		return fmt.Errorf("expected empty line but got %v", scanner.Bytes())
	}

	// Get result code
	read = scanner.Scan()
	if !read {
		return fmt.Errorf("resultCode - unexpected end of input")
	}

	resultCode := scanner.Text()
	if resultCode != AtReplyOk {
		m.gpsMode = atparser.GpsModeUnknown
		return fmt.Errorf("modem did not reply with AT OK got: %v -> %s", []byte(resultCode), resultCode)
	}

	// Get the actual mode from the device
	started, mode, err := atparser.GPSStatus(modeResult)

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

// StartGPS Triest to start the GPS, only standalone mode is supported
func (m *Modem) StartGPS(desiredMode atparser.GPSModeEnum, forceRestart bool) error {
	if desiredMode != atparser.GpsModeStandalone {
		return errors.New("unsupported GPS mode, only standalone supported for now")
	}

	initError := m.initialized()
	if initError != nil {
		return initError
	}

	// Retrieve the current gps mode
	err := m.determineGPSMode()
	if err != nil {
		return err
	}

	// If the gps is not offline but in a mode thats not standalone, reset
	gpsStopInitiated := false
	if m.gpsMode != atparser.GpsModeOffline && m.gpsMode != desiredMode {
		// Reset the gps
		err = m.StopGPS()
		if err != nil {
			log.Error("Failed to stop gps", zap.String("error", err.Error()))
			return err
		}
		gpsStopInitiated = true

		// #todo improvement: the device prints +CGPS: 0 after some time,
		// so we could theoretically poll here and block until the reset commences
	}

	// Dont start gps because we are already up and running
	if m.gpsStarted && m.gpsMode == desiredMode {
		log.Debug("GPS already started", zap.String("mode", string(m.gpsMode)))

		// If we dont force a restart, we can exit here
		if !forceRestart {
			return nil
		}

		err := m.StopGPS()
		if err != nil {
			return err
		}

		// fixme: sleeping 5 seconds is not what we need here, block & wait somehow
		time.Sleep(5 * time.Second)
	}

	// Send GPS Start command
	err = m.writeSerialWithResult(AtGpsState+"=1,"+string(desiredMode), AtReplyOk)
	if err != nil {
		// If we recently stopped the GPS, this error is to be expected
		if gpsStopInitiated {
			return &GPSNotYetReadyError{}
		} else {
			return err
		}
	}

	// Re-run mode check to verify that the modem started in the mode we wanted
	err = m.determineGPSMode()
	if err != nil {
		return err
	}

	if desiredMode != m.gpsMode {
		log.Error("GPS mode mismatch while starting", zap.String("wanted", string(desiredMode)), zap.String("got", string(m.gpsMode)))
	}

	log.Info("GPS started", zap.String("mode", string(m.gpsMode)))
	return nil
}

func (m *Modem) StopGPS() error {
	initError := m.initialized()
	if initError != nil {
		return initError
	}

	return m.writeSerialWithResult(AtGpsState+"=0", AtReplyOk)
}

func (m *Modem) GetGPSState() (bool, error) {
	initError := m.initialized()
	if initError != nil {
		return false, initError
	}

	return true, nil
}

func (m *Modem) ResetGPS() error {
	initError := m.initialized()
	if initError != nil {
		return initError
	}

	return nil
}

// Enable Stub for Sim7600
func (m *Modem) Enable() error {
	return nil
}

// Disable Stub for Sim7600
func (m *Modem) Disable() error {
	return nil
}
