package client

import (
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/config"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/sensors"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/gnss"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/rauc"
	"disco.cs.uni-kl.de/apogee/pkg/systemd"
	"disco.cs.uni-kl.de/apogee/pkg/usb"
	"go.uber.org/zap"
)

const (
	ProductName             = "apogee"
	UserdataDirectoryPrefix = "/data/"
	ConfigFolder            = "config/"

	ConfigPathPrefix = ConfigFolder + ProductName + "/"
	ConfigFile       = "config.toml"
	CertFile         = "discosat.crt"

	DefaultConfigPath = UserdataDirectoryPrefix + ConfigPathPrefix + ConfigFile
	DefaultRootCert   = UserdataDirectoryPrefix + ConfigPathPrefix + CertFile

	DefaultTmpDir = "/run/" + ProductName + "/tmp/"

	DefaultJobStorageDir   = UserdataDirectoryPrefix + "jobs/"
	DefaultJobTmpDir       = DefaultTmpDir + "jobs/"
	DefaultPollingInterval = time.Second * 30

	DefaultDebugModeValue = false
)

type CLIFlags struct {
	ConfigPath string
	RootCert   string
	Debug      bool
}

// App global app struct that contains all services
type App struct {
	// A global wait group, all go routines that should
	// terminate when the application ends should be registered here
	WG sync.WaitGroup

	ReloadSignal chan os.Signal
	ExitSignal   chan os.Signal

	// The API
	Api *api.RestAPI

	// The CLIFlags passed to the application
	CliFlags *CLIFlags
	Config   *config.Config

	SystemdConnector *systemd.Connector

	OtaService     rauc.Service
	GNSSService    gnss.Service
	NetworkService net.NetworkService
	UsbManager     *usb.USBDeviceManager
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

func (a *App) SensorName() string {
	return a.Config.Api.SensorName
}

func ParseCLIFlags() *CLIFlags {
	flags := &CLIFlags{}
	flag.StringVar(&flags.ConfigPath, "config", DefaultConfigPath, "relative or absolute path to the config file")
	flag.StringVar(&flags.RootCert, "rootcert", DefaultRootCert, "relative or absolute path to the root certificate used for server validation")
	flag.BoolVar(&flags.Debug, "debug", DefaultDebugModeValue, "true if the debug logging should be enabled")
	flag.Parse()

	return flags
}

func setDefaults(config *config.Config, flags *CLIFlags) (*config.Config, error) {
	// If the cert specified on the cli is not the default one, use it instead
	if config.Api.Security.RootCertificate == "" || flags.RootCert != DefaultRootCert {
		config.Api.Security.RootCertificate = flags.RootCert
	}

	// Set up the default directories
	if config.Jobs.StoragePath == "" {
		config.Jobs.StoragePath = DefaultJobStorageDir
	}

	if config.Jobs.TempPath == "" {
		config.Jobs.TempPath = DefaultJobTmpDir
	}

	// This prevents zero polling intervals
	if config.Jobs.PollingInterval == 0 {
		config.Jobs.PollingInterval = DefaultPollingInterval
	}

	return config, nil
}

func loadConfiguration(app *App) error {
	flags := app.CliFlags
	configPath := flags.ConfigPath

	var err error

	// Check given configFile
	if err = config.ValidatePath(configPath); err != nil {
		log.Error("error while loading configuration: " + err.Error())

		// Fallback to default
		configPath = DefaultConfigPath
		if err = config.ValidatePath(configPath); err != nil {
			log.Error("all possible configuration paths exhausted, falling back to built-in defaults", zap.Error(err))
		}
	}

	// Decode config file, no field validation is taking place here, the code using it is required to check for required fields etc.
	app.Config, err = config.NewConfiguration(configPath)
	if err != nil {
		log.Error("an error occurred while trying to load the config file", zap.String("path", configPath), zap.Error(err))
		app.Config = &config.Config{}
	}

	// Set the defaults in case the user omitted some fields
	if _, err = setDefaults(app.Config, app.CliFlags); err != nil {
		log.Error("config defaults could not be set")
		return err
	}

	// Check given certFile
	if err := config.ValidatePath(app.Config.Api.Security.RootCertificate); err != nil {
		log.Warn("error while loading certificate", zap.Error(err))
	}

	// fixme this should be sanitized to not leak secrets
	log.Debug("Active configuration", zap.Any("config", *app.Config))
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
	if !instrumentation {
		app.CliFlags = ParseCLIFlags()
	} else {
		app.CliFlags = &CLIFlags{Debug: true}
	}

	// Register a quit signal
	app.ExitSignal = make(chan os.Signal, 1)
	signal.Notify(app.ExitSignal, os.Interrupt, syscall.SIGTERM)

	// Register the reload signal
	app.ReloadSignal = make(chan os.Signal, 1)
	signal.Notify(app.ReloadSignal, syscall.SIGUSR1, syscall.SIGUSR2)

	// Initialize logger
	log.Init(app.CliFlags.Debug)

	// removeme: temporary playground
	temps := sensors.ReadTemperatures()
	log.Info("tempSensors", zap.Any("sensors", temps))

	log.Info("apogeeclient starting")

	// Load the configuration file
	err := loadConfiguration(&app)

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
		log.Fatal("Could not initialize system dbus connection and required services", zap.Error(err))
	}

	if !instrumentation {
		// Set up the remote API
		app.Api, err = api.NewRestAPI(app.Config.Api)
		if err != nil {
			log.Panic("Could not initialize api, aborting", zap.Error(err))
		}
	}

	// Setup usb and run the SDR scan
	app.UsbManager = usb.NewUSBDeviceManager()
	devices := app.UsbManager.FindSupportedDevices()
	if devices != nil {
		log.Error("result reset", zap.Error(app.UsbManager.ResetDevice(usb.SDRHackRFOne)))
	}

	// If api setup fails and we are not in local mode, terminate application
	if err != nil {
		return &app, err
	}

	return &app, nil
}
