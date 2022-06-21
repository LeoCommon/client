package gps

import (
	"fmt"
	"math"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"go.uber.org/zap"
)

type BackendType int32

const (
	// Stub implementation
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

// # interface methods
type GPSService interface {
	initialize() error
	GetData() GPSData
	Shutdown()
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

// Invalid GPSData has no proper timestamp
func (d GPSData) Valid() bool {
	return d.Time != 0 && !math.IsNaN(d.Time)
}

func NewService(backend BackendType, arguments interface{}) (GPSService, error) {
	var service GPSService

	switch backend {
	case GPSD:
		args, ok := arguments.(*GpsdBackendParameters)
		if !ok {
			return nil, fmt.Errorf("unsupported gpsd backend parameter type")
		}
		service = &gpsdService{args: args}
	case STUB:
		service = &gpsStubService{}
	}

	// Initialize service
	err := service.initialize()

	if err != nil {
		return nil, err
	}

	apglog.Info("GPS Backend selected:", zap.String("name", backend.String()))

	return service, nil
}
