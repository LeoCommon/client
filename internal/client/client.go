package client

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/LeoCommon/client/internal/client/api"
	"github.com/LeoCommon/client/internal/client/config"
	"github.com/LeoCommon/client/pkg/log"
	"github.com/LeoCommon/client/pkg/system/sensors"
	"github.com/LeoCommon/client/pkg/system/services/gnss"
	"github.com/LeoCommon/client/pkg/system/services/net"
	"github.com/LeoCommon/client/pkg/system/services/rauc"
	"github.com/LeoCommon/client/pkg/systemd"
	"github.com/LeoCommon/client/pkg/usb"
	"go.uber.org/zap"
)

// App global app struct that contains all services
type App struct {
	// A global wait group, all go routines that should
	// terminate when the application ends should be registered here
	WG sync.WaitGroup

	ReloadSignal chan os.Signal
	ExitSignal   chan os.Signal

	// The API
	Api *api.RestAPI

	Conf *config.Manager

	SystemdConnector *systemd.Connector

	OtaService     rauc.Service
	GNSSService    gnss.Service
	NetworkService net.NetworkService
	UsbManager     *usb.USBDeviceManager
	TestRunning    bool
}

func (a *App) Shutdown() {
	if a.GNSSService != nil {
		a.GNSSService.Shutdown()
	}

	if a.OtaService != nil {
		a.OtaService.Shutdown()
	}

	if a.NetworkService != nil {
		a.NetworkService.Shutdown()
	}

	// Close the systemd connector
	if a.SystemdConnector != nil {
		_ = a.SystemdConnector.Shutdown()
	}

	if a.UsbManager != nil {
		a.UsbManager.Shutdown()
	}
}

func (a *App) loadConfiguration(configPath string, rootCert string, acceptEmptyConfig bool) error {
	// Create the new config manager and load the configuration
	a.Conf = config.NewManager()
	if err := a.Conf.Load(configPath, acceptEmptyConfig); err != nil {
		log.Error("an error occurred while trying to load the config file, trying default path", zap.String("path", configPath), zap.Error(err))
		err = a.Conf.Load(config.DefaultConfigPath, acceptEmptyConfig)
		if err != nil {
			// Only terminate if empty configs are not okay
			if !acceptEmptyConfig {
				return err
			}
		}
	}

	// Allow overwriting the root certificate
	if len(rootCert) != 0 {
		a.Conf.Api().Set(func(param *config.ApiConfig) {
			param.RootCertificate = rootCert
		})
	}

	return nil
}

func startGPSService(app *App) {
	// Start GPSD Monitor, fall back to stub if a startup failure happened
	gpsService, err := gnss.NewService(gnss.GPSD, app.SystemdConnector)
	if err != nil {
		log.Error("Could not initialize gpsd data backend, falling back to stub", zap.Error(err))
		gpsService, _ = gnss.NewService(gnss.STUB, app.SystemdConnector)
	}

	log.Info("Location received", zap.String("data", gpsService.GetData().String()))
	app.GNSSService = gpsService
}

// Sets up the updater service
// This only supports RAUC for now but i
func setupOTAService(app *App) {
	otaService, err := rauc.NewService(app.SystemdConnector)
	if err != nil {
		log.Error("OTA Service could not be initialized", zap.Error(err))
		// todo: implement stub for platforms without OTA support
	}

	app.OtaService = otaService
}

func startNetworkService(app *App) {
	nsvc, err := net.NewService(app.SystemdConnector)
	if err != nil {
		log.Error("Network service could not be started", zap.Error(err))
	}

	app.NetworkService = nsvc
}

func Setup(instrumentation bool) (*App, error) {
	app := App{}

	// Skip cli flag parsing on testing
	var flags config.CLIFlags
	if !instrumentation {
		flags = config.ParseCLIFlags()
	} else {
		flags = config.CLIFlags{Debug: true}
		app.TestRunning = instrumentation
	}

	// Register a quit signal
	app.ExitSignal = make(chan os.Signal, 1)
	signal.Notify(app.ExitSignal, os.Interrupt, syscall.SIGTERM)

	// Register the reload signal
	app.ReloadSignal = make(chan os.Signal, 1)
	signal.Notify(app.ReloadSignal, syscall.SIGUSR1, syscall.SIGUSR2)

	// Initialize logger
	log.Init(flags.Debug)

	log.Info("client starting")

	// Load the configuration file
	err := app.loadConfiguration(flags.ConfigPath, flags.RootCert, instrumentation)
	if err != nil {
		if !instrumentation {
			app.Shutdown()
			return nil, err
		}

		// reset the error if we are running a test
		err = nil
	}

	// Dont connect to dbus when testing
	if !instrumentation {
		// Connect to systemd & dbus
		app.SystemdConnector, err = systemd.NewConnector()
		if err != nil {
			log.Warn("could not connect to dbus, all related functionality is disabled.", zap.Error(err))
		}
	}

	if err == nil {
		// Start GPSService
		startGPSService(&app)
		// Prepare OTAService
		setupOTAService(&app)
		// Prepare network Service
		startNetworkService(&app)
	} else {
		app.Shutdown()
		log.Error("Could not initialize system dbus connection and required services", zap.Error(err))
		return nil, err
	}

	if !instrumentation {
		// Set up the remote API
		app.Api, err = api.NewRestAPI(app.Conf, flags.Debug)
		if err != nil {
			app.Shutdown()
			log.Error("Could not initialize api, aborting", zap.Error(err))
			return &app, err
		}
	}

	// Setup usb and run the device scan to get startup output
	app.UsbManager = usb.NewUSBDeviceManager()
	app.UsbManager.FindSupportedDevices()

	// Output all system temperatures
	log.Info("system_temperatures", zap.Any("sensors", sensors.ReadTemperatures()))

	return &app, err
}
