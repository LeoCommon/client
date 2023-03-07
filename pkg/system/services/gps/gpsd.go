package gps

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/constants"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"

	dbu "github.com/coreos/go-systemd/v22/dbus"
)

const (
	// Set to true, to get output on each signal fix
	DEBUG_GPSD = false

	GPSD_DBUS_OBJECT_PATH     = "/org/gpsd"
	GPSD_DBUS_INTERFACE       = "org.gpsd"
	GPSD_DBUS_FIX_SIGNAL_NAME = GPSD_DBUS_INTERFACE + ".fix"

	GPSD_DATA_MAX_SIGNAL_AGE = 5000 // 5000 ms is 5 seconds of maximal signal age
	GPSD_SYSTEMD_UNIT_NAME   = "gpsd.service"

	GPSD_INITIAL_FIX_RETRIES   = 6     // Try 3 times to get a fix signal from gpsd before giving up
	GPSD_SYNC_RETRIEVE_TIMEOUT = 10000 // Wait 10 seconds for signals to be received in sync mode
)

func (s *gpsdService) GetData() GPSData {
	s.dataLock.L.Lock()
	defer s.dataLock.L.Unlock()

	return s.data
}

func assignIfNaN(src *float64, dst float64) {
	if src == nil {
		log.Error("`nil` ptr passed, aborting")
		return
	}

	if math.IsNaN(*src) {
		*src = dst
	}
}

// This function ammends the data from one gpsdfix signal with another one if the time is identical
func updateOrAmmendGPSData(s1 *GPSData, s2 *GPSDFixSignal) {
	// <Martin Böh> We know the time is unix timestamps so no fractional component is to be expected
	// Time stamp is not equal or its completely new data, do not append
	if math.IsNaN(s1.Time) || math.IsNaN(s2.Time) || int64(s1.Time) != int64(s2.Time) {
		s1.Time = s2.Time
		s1.Lat = s2.Lat
		s1.Lon = s2.Lon
		s1.AltMSL = s2.AltMSL
		s1.Speed = s2.Speed
		return
	}

	// If some of the previously received data is NaN, try to fill it
	assignIfNaN(&s1.Lat, s2.Lat)
	assignIfNaN(&s1.Lon, s2.Lon)
	assignIfNaN(&s1.AltMSL, s2.AltMSL)
	assignIfNaN(&s1.Speed, s2.Speed)
}

func (s *gpsdService) satelliteObjectReceiver() {
	for v := range s.signalCh.C {
		if v.Name == GPSD_DBUS_FIX_SIGNAL_NAME {
			fix, err := parseGPSDSatelliteObject(v.Body)
			if err != nil {
				log.Error("Received invalid signal data", zap.Any("data", v.Body))
				continue
			}

			// Assign received signal data to our own data field
			s.dataLock.L.Lock()

			reliable := IsDataReliable(fix)

			if DEBUG_GPSD {
				log.Debug("Received location data from gpsd", zap.Bool("reliable", reliable), zap.String("device", fix.DeviceName),
					zap.Float64("time", fix.Time), zap.Float64("lat", fix.Lat), zap.Float64("lon", fix.Lon),
					zap.Float64("altMSL", fix.AltMSL))
			}

			updateOrAmmendGPSData(&s.data, fix)

			// Signal the change if anyone is waiting for initial data
			s.dataLock.Broadcast()
			s.dataLock.L.Unlock()
		}
	}

	log.Debug("onGPSDFixEvent channel terminated")
}

func restartGPSDDaemon(conn *dbu.Conn) error {
	log.Info("(Re)starting gpsd daemon")

	reschan := make(chan string)
	_, err := conn.RestartUnitContext(context.Background(), GPSD_SYSTEMD_UNIT_NAME, "replace", reschan)
	if err != nil {
		return err
	}

	job := <-reschan
	if job != constants.SYSTEMD_SERVICE_START_RESULT_DONE {
		return fmt.Errorf(job)
	}

	return nil
}

func (s *gpsdService) waitForGPSFixOrTimeout() bool {
	done := make(chan struct{})
	go func() {
		for !s.IsGPSTimeValid() {
			// looks stupid, but is necessary to handle the locks inside the Wait()
			s.dataLock.L.Lock()
			s.dataLock.Wait()
			s.dataLock.L.Unlock()
		}

		close(done)
	}()

	select {
	case <-time.After(GPSD_SYNC_RETRIEVE_TIMEOUT * time.Millisecond):
		log.Warn("timeout while waiting for gpsd satellite object")
		return false
	case <-done:
		return true
	}
}

// Prepare(Start/Reset) and validate receiving of the gpsd daemon if its not running
func (s *gpsdService) prepareAndValidateGPSD() error {
	// Establish a new system connection
	conn, _ := dbu.NewSystemConnectionContext(context.Background())
	defer conn.Close()

	// Try to start the service GPSD_INITIAL_FIX_RETRIES times
	for i := 0; i < GPSD_INITIAL_FIX_RETRIES; i++ {
		// Get the gpsd systemd service
		stati, _ := conn.ListUnitsByNamesContext(context.Background(), []string{GPSD_SYSTEMD_UNIT_NAME})
		if len(stati) == 0 {
			return fmt.Errorf("no gpsd systemd service found, aborting")
		}

		// Find the unit status in the list and check its state
		serviceStarted := false
		for _, status := range stati {
			if status.ActiveState == constants.SYSTEMD_SERVICE_STATE_ACTIVE {
				serviceStarted = true
				break
			}
		}

		// Restart the service if it's not active or if this is not our first attempt
		// Technically this should be handled by udev.d events within the OS but better safe than sorry
		if !serviceStarted || i > 0 {
			err := restartGPSDDaemon(conn)

			// We failed to start the daemon, this is likely unfixable, don't retry!
			if err != nil {
				return err
			}
		}

		// Start listening for signals and buffer up to 10 events
		s.signalCh = &dbusSignalChannel{C: make(chan *dbus.Signal, 10)}
		s.args.Conn.Signal(s.signalCh.C)
		go s.satelliteObjectReceiver()

		// Service should be active by now!
		// If it is not, the timeout will trigger, and we will retry
		if s.waitForGPSFixOrTimeout() {
			go s.gpsdWatchdog(conn)
			return nil
		}

		// Remove the signal match and close the channel on error
		s.resetSignalChannel()
	} // Retry

	return fmt.Errorf("failed to acquire gpsd satellite object")
}

func (s *gpsdService) initialize() error {
	if s.args.Conn == nil {
		return fmt.Errorf("dbus connection not available")
	}

	s.watchGPS = false
	// Specify a NaN time to signal that no valid data exists!
	s.data.Time = math.NaN()

	// Create an initial "dataLock" hook condition
	dataLock := sync.Mutex{}
	s.dataLock = sync.NewCond(&dataLock)

	// Register the new gpsd dbus matchers
	s.dbusMatchOptions = []dbus.MatchOption{dbus.WithMatchObjectPath(GPSD_DBUS_OBJECT_PATH),
		dbus.WithMatchInterface(GPSD_DBUS_INTERFACE)}

	// Set up the matcher before anything else, we need this only once
	if err := s.args.Conn.AddMatchSignal(s.dbusMatchOptions...); err != nil {
		return err
	}

	return s.prepareAndValidateGPSD()
}

func (s *gpsdService) Shutdown() {
	s.watchGPS = false
	// Remove the matchers
	s.args.Conn.RemoveMatchSignal(s.dbusMatchOptions...)
	s.resetSignalChannel()
}

func (s *gpsdService) resetSignalChannel() {
	// Remove and stop the signal channel
	s.args.Conn.RemoveSignal(s.signalCh.C)

	// Only close the channel once
	s.signalCh.once.Do(func() {
		close(s.signalCh.C)
	})
}

type dbusSignalChannel struct {
	once sync.Once
	C    chan *dbus.Signal
}

type gpsdService struct {
	args             *GpsdBackendParameters
	signalCh         *dbusSignalChannel
	dataLock         *sync.Cond
	dbusMatchOptions []dbus.MatchOption
	data             GPSData
	watchGPS         bool
}

type GpsdBackendParameters struct {
	Conn *dbus.Conn
}

type GPSDFixSignal struct {
	DeviceName        string
	AltMSL            float64
	Course            float64
	Lat               float64
	Lon               float64
	HorizUncertainty  float64
	Time              float64
	AltUncertainty    float64
	TimeUncertainty   float64
	CourseUncertainty float64
	Speed             float64
	SpeedUncertainty  float64
	Climb             float64
	ClimbUncertainty  float64
	Mode              int32
}

func IsGPSTimeValid(timestamp float64) bool {
	return timestamp != 0 && !math.IsNaN(timestamp)
}

func (s *gpsdService) IsGPSTimeValid() bool {
	s.dataLock.L.Lock()
	defer s.dataLock.L.Unlock()
	return s.data.Time != 0 && !math.IsNaN(s.data.Time)
}

func IsDataReliable(fs *GPSDFixSignal) bool {
	if !IsGPSTimeValid(fs.Time) {
		return false
	}

	diff := (time.Now().Unix() - int64(fs.Time))

	// If the diff is below 0 the gps data is newer than our system time
	if diff < -1 {
		log.Warn("GPS Data Time Drift detected", zap.Int64("drift", diff))
		return false
	}

	return diff < GPSD_DATA_MAX_SIGNAL_AGE
}

/*
Parses a raw dbus body to a GPSDFixSignal
For reference: https://gpsd.gitlab.io/gpsd/gpsd.html#_shared_memory_and_dbus_interfaces
*/
func parseGPSDSatelliteObject(v []interface{}) (*GPSDFixSignal, error) {
	if v == nil {
		return nil, fmt.Errorf("received satellite object was nil")
	}

	const SAT_OBJECT_LENGTH = 15
	if len(v) != SAT_OBJECT_LENGTH {
		return nil, fmt.Errorf("malformed satellite object received length %d != %d", len(v), SAT_OBJECT_LENGTH)
	}

	var ok bool
	fix := &GPSDFixSignal{}

	fix.Time, ok = v[0].(float64)
	if !ok {
		return nil, fmt.Errorf("time could not be interpreted as float64")
	}

	fix.Mode, ok = v[1].(int32)
	if !ok {
		return nil, fmt.Errorf("mode could not be interpreted as int32")
	}

	fix.TimeUncertainty = v[2].(float64)
	if !ok {
		return nil, fmt.Errorf("timeuncertainty could not be interpreted as float64")
	}

	fix.Lat = v[3].(float64)
	if !ok {
		return nil, fmt.Errorf("lat could not be interpreted as float64")
	}

	fix.Lon = v[4].(float64)
	if !ok {
		return nil, fmt.Errorf("lon could not be interpreted as float64")
	}

	fix.HorizUncertainty = v[5].(float64)
	if !ok {
		return nil, fmt.Errorf("horizuncertainty could not be interpreted as float64")
	}

	fix.AltMSL = v[6].(float64)
	if !ok {
		return nil, fmt.Errorf("altmsl could not be interpreted as float64")
	}

	fix.AltUncertainty = v[7].(float64)
	if !ok {
		return nil, fmt.Errorf("altuncertainty could not be interpreted as float64")
	}

	fix.Course = v[8].(float64)
	if !ok {
		return nil, fmt.Errorf("course could not be interpreted as float64")
	}

	fix.CourseUncertainty = v[9].(float64)
	if !ok {
		return nil, fmt.Errorf("courseuncertainty could not be interpreted as float64")
	}

	fix.Speed = v[10].(float64)
	if !ok {
		return nil, fmt.Errorf("speed could not be interpreted as float64")
	}

	fix.SpeedUncertainty = v[11].(float64)
	if !ok {
		return nil, fmt.Errorf("speeduncertainty could not be interpreted as float64")
	}

	fix.Climb = v[12].(float64)
	if !ok {
		return nil, fmt.Errorf("climb could not be interpreted as float64")
	}

	fix.ClimbUncertainty = v[13].(float64)
	if !ok {
		return nil, fmt.Errorf("climbuncertainty could not be interpreted as float64")
	}

	fix.DeviceName = v[14].(string)
	if !ok {
		return nil, fmt.Errorf("devicename could not be interpreted as string")
	}

	return fix, nil
}

// start the watchdog in a separate go-routine
func (s *gpsdService) gpsdWatchdog(conn1 *dbu.Conn) {
	// In case of a short connection loss of the modem (as USB-cable-problem), gpsd has to be reset.
	log.Info("gpsd watchdog started")
	s.watchGPS = true
	lastGPStime := 0.0
	for s.watchGPS {
		// sleep a while
		time.Sleep(GPSD_SYNC_RETRIEVE_TIMEOUT * time.Millisecond)
		currentTime := s.GetData().Time
		if s.IsGPSTimeValid() && lastGPStime != currentTime {
			// if everything is okay, sleep again
			lastGPStime = currentTime
			continue
		}
		log.Info("gpsd watchdog detected an anomaly")
		// try restart gpsd-daemon
		err := restartGPSDDaemon(conn1)
		if err == nil {
			continue
		}

		if strings.Contains(err.Error(), "connection closed") {
			// this is expected. Establish a new system connection.
			conn2, _ := dbu.NewSystemConnectionContext(context.Background())
			defer conn2.Close()

			err = restartGPSDDaemon(conn2)
			if err != nil {
				log.Error("Watchdog could not restart GPSD, even after restarting the dbus-connection", zap.Error(err))
			}
			conn1 = conn2
		} else {
			log.Error("Watchdog could not restart GPSD", zap.Error(err))
		}
	}
}
