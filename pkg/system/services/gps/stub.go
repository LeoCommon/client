package gps

import (
	"time"
)

func (s *gpsStubService) GetData() GPSData {
	s.gpsData.Time = float64(time.Now().Unix())
	return s.gpsData
}
func (s *gpsStubService) initialize() error {
	s.gpsData.AltMSL = 100
	s.gpsData.Lat = 49.42469
	s.gpsData.Lon = 7.75094
	s.gpsData.Speed = 0.0

	return nil
}

func (s *gpsStubService) Shutdown() {
	// stub
}

type gpsStubService struct {
	gpsData GPSData
}
