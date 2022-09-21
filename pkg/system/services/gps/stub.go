package gps

import (
	"time"
)

func (s *gpsStubService) GetData() GPSData {
	s.gpsData.Time = float64(time.Now().Unix())
	return s.gpsData
}
func (s *gpsStubService) initialize() error {
	s.gpsData.AltMSL = 0
	s.gpsData.Lat = 0
	s.gpsData.Lon = 0
	s.gpsData.Speed = 0.0

	return nil
}

func (s *gpsStubService) Shutdown() {
	// stub
}

func (s *gpsStubService) IsGPSTimeValid() bool {
	return false
}

type gpsStubService struct {
	gpsData GPSData
}
