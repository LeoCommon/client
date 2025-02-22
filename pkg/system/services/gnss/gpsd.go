package gnss

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/LeoCommon/client/pkg/log"
	"github.com/LeoCommon/client/pkg/systemd"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

const (
	DebugGpsd = false

	GpsdDbusObjectPath    = "/org/gpsd"
	GpsdDbusInterface     = "org.gpsd"
	GpsdDbusFixSignalName = GpsdDbusInterface + ".fix"

	GpsdDataMaxSignalAge = 5000 // 5000 ms is 5 seconds of maximal signal age
	GpsdSystemdUnitName  = "gpsd.service"

	GpsdInitialFixRetries   = 6     // Try 3 times to get a fix signal from gpsd before giving up
	GpsdSyncRetrieveTimeout = 10000 // Wait 10 seconds for signals to be received in sync mode
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
		if v.Name == GpsdDbusFixSignalName {
			fix, err := parseGPSDSatelliteObject(v.Body)
			if err != nil {
				log.Error("Received invalid signal data", zap.Any("data", v.Body))
				continue
			}

			// Assign received signal data to our own data field
			s.dataLock.L.Lock()

			reliable := IsDataReliable(fix)

			if DebugGpsd {
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

func (s *gpsdService) restartGPSDDaemon() error {
	log.Info("(Re)starting gpsd daemon")

	success, err := s.systemd.RestartUnit(GpsdSystemdUnitName, context.Background())
	if err != nil {
		return err
	}

	if success {
		return nil
	}

	return fmt.Errorf("service restart not successful")
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
	case <-time.After(GpsdSyncRetrieveTimeout * time.Millisecond):
		log.Warn("timeout while waiting for gpsd satellite object")
		return false
	case <-done:
		return true
	}
}

// Prepare(Start/Reset) and validate receiving of the gpsd daemon if its not running
func (s *gpsdService) prepareAndValidateGPSD() error {
	// Try to start the service GPSD_INITIAL_FIX_RETRIES times
	for i := 0; i < GpsdInitialFixRetries; i++ {
		state, err := s.systemd.CheckUnitState(GpsdSystemdUnitName, context.Background())
		// Error can only happen if the service does not exist or theres connection issues
		if err != nil {
			return err
		}

		// Restart the service if it's not active or if this is not our first attempt
		// Technically this should be handled by udev.d events within the OS but better safe than sorry
		if state != systemd.ServiceStateActive || i > 0 {
			err := s.restartGPSDDaemon()

			// We failed to start the daemon, this is likely unfixable, don't retry!
			if err != nil {
				return err
			}
		}

		// Start listening for signals and buffer up to 10 events
		s.signalCh = &dbusSignalChannel{C: make(chan *dbus.Signal, 10)}
		s.systemd.Signal(s.signalCh.C)
		go s.satelliteObjectReceiver()

		// Service should be active by now!
		// If it is not, the timeout will trigger, and we will retry
		if s.waitForGPSFixOrTimeout() {
			go s.gpsdWatchdog()
			return nil
		}

		// Remove the signal match and close the channel on error
		s.resetSignalChannel()
	} // Retry

	return fmt.Errorf("failed to acquire gpsd satellite object")
}

func (s *gpsdService) initialize() error {
	// Verify required services
	if s.systemd == nil || !s.systemd.Connected() {
		return fmt.Errorf("required connectivity not available")
	}

	s.watchGPS = false
	// Specify a NaN time to signal that no valid data exists!
	s.data.Time = math.NaN()

	// Create an initial "dataLock" hook condition
	dataLock := sync.Mutex{}
	s.dataLock = sync.NewCond(&dataLock)

	// Register the new gpsd dbus matchers
	s.dbusMatchOptions = []dbus.MatchOption{dbus.WithMatchObjectPath(GpsdDbusObjectPath),
		dbus.WithMatchInterface(GpsdDbusInterface)}

	// Set up the matcher before anything else, we need this only once
	if err := s.systemd.AddMatchSignal(s.dbusMatchOptions...); err != nil {
		log.Error("could not add match signal", zap.Error(err))
		return err
	}

	return s.prepareAndValidateGPSD()
}

func (s *gpsdService) Shutdown() error {
	s.watchGPS = false
	// Remove the matchers

	err := s.systemd.RemoveMatchSignal(s.dbusMatchOptions...)
	if err != nil {
		log.Error("could not remove match signal", zap.Error(err))
		return err
	}
	s.resetSignalChannel()
	return nil
}

func (s *gpsdService) resetSignalChannel() {
	// Remove and stop the signal channel
	s.systemd.RemoveSignal(s.signalCh.C)

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
	systemd          *systemd.Connector
	signalCh         *dbusSignalChannel
	dataLock         *sync.Cond
	dbusMatchOptions []dbus.MatchOption
	data             GPSData
	watchGPS         bool
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

	diff := time.Now().Unix() - int64(fs.Time)

	// If the diff is below 0 the gps data is newer than our system time
	if diff < -1 {
		log.Warn("GPS Data Time Drift detected", zap.Int64("drift", diff))
		return false
	}

	return diff < GpsdDataMaxSignalAge
}

/*
Parses a raw dbus body to a GPSDFixSignal
For reference: https://gpsd.gitlab.io/gpsd/gpsd.html#_shared_memory_and_dbus_interfaces
*/
func parseGPSDSatelliteObject(v []interface{}) (*GPSDFixSignal, error) {
	if v == nil {
		return nil, fmt.Errorf("received satellite object was nil")
	}

	const SatObjectLength = 15
	if len(v) != SatObjectLength {
		return nil, fmt.Errorf("malformed satellite object received length %d != %d", len(v), SatObjectLength)
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
func (s *gpsdService) gpsdWatchdog() {
	// In case of a short connection loss of the modem (as USB-cable-problem), gpsd has to be reset.
	log.Info("gpsd watchdog started")
	s.watchGPS = true
	lastGPStime := 0.0
	for s.watchGPS {
		// sleep a while
		time.Sleep(GpsdSyncRetrieveTimeout * time.Millisecond)
		currentTime := s.GetData().Time
		if s.IsGPSTimeValid() && lastGPStime != currentTime {
			// if everything is okay, sleep again
			lastGPStime = currentTime
			continue
		}

		log.Warn("gpsd watchdog detected a time anomaly")
		// try restarting gpsd-daemon
		err := s.restartGPSDDaemon()
		if err == nil {
			continue
		}

		// If there is an error, we expect it to be a closed dbus connection
		if errors.Is(err, dbus.ErrClosed) {
			log.Error("Dbus connection closed, waiting for re-connect.", zap.Error(err))

			// Try to reconnect, result does not matter
			err = s.systemd.RequestReconnect()
			if err != nil {
				log.Error("reconnect attempt failed", zap.Error(err))
			}
			continue
		}

		log.Error("unknown error while trying to restart gpsd", zap.Error(err))
	}
}
