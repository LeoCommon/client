package gnss

import (
	"fmt"
	"math"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/systemd"
	"go.uber.org/zap"
)

type BackendType int32

const (
	// STUB implementation
	STUB BackendType = -1

	// GPSD based
	GPSD BackendType = 0
)

func (e BackendType) String() string {
	switch e {
	case STUB:
		return "StubImplementation"
	case GPSD:
		return "GPSDDbus"
	default:
		return fmt.Sprintf("%d", int(e))
	}
}

// Service interface methods
type Service interface {
	initialize() error
	GetData() GPSData
	Shutdown() error
	IsGPSTimeValid() bool
}

// The GPSData that is needed by the application
type GPSData struct {
	// Time is supposed to come from the GPS, do not set if no valid gps data was received
	Time   float64
	Lat    float64
	Lon    float64
	AltMSL float64
	Speed  float64
}

func (d GPSData) String() string {
	return fmt.Sprintf("GPSData(%d) valid: %v - lat: %f lon: %f alt: %f speed: %f",
		int64(d.Time), d.Valid(), d.Lat, d.Lon, d.AltMSL, d.Speed)
}

// Valid checks if the GPSData is valid, Invalid data has no proper timestamp
func (d GPSData) Valid() bool {
	return d.Time != 0 && !math.IsNaN(d.Time)
}

func NewService(backend BackendType, systemd *systemd.Connector) (Service, error) {
	var service Service

	switch backend {
	case GPSD:
		service = &gpsdService{systemd: systemd}
	case STUB:
		service = &gpsStubService{}
	}

	// Initialize service
	err := service.initialize()

	if err != nil {
		return nil, err
	}

	log.Info("GPS Backend selected:", zap.String("name", backend.String()))

	return service, nil
}
