package apogee

import (
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/config"
	"disco.cs.uni-kl.de/apogee/pkg/system/bus"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/gps"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/rauc"
	"go.uber.org/zap"
)

var (
	PRODUCT_NAME              = "apogee"
	USERDATA_DIRECTORY_PREFIX = "/data/"
	CONFIG_FOLDER             = "config/"

	CONFIG_PATH_PREFIX = CONFIG_FOLDER + PRODUCT_NAME + "/"
	CONFIG_FILE        = "config.yml"
	CERT_FILE          = "discosat.crt"

	DEFAULT_CONFIG_PATH = USERDATA_DIRECTORY_PREFIX + CONFIG_PATH_PREFIX + CONFIG_FILE
	DEFAULT_ROOT_CERT   = USERDATA_DIRECTORY_PREFIX + CONFIG_PATH_PREFIX + CERT_FILE

	// Default temp folders
	DEFAULT_TMP_DIR     = "/run/" + PRODUCT_NAME + "/tmp/"
	DEFAULT_JOB_TMP_DIR = DEFAULT_TMP_DIR + "jobs/"

	// #fixme this could move to /data/jobs/ as its not related to config, needs satos fixes though
	DEFAULT_JOB_COLLECT_DIR = USERDATA_DIRECTORY_PREFIX + CONFIG_PATH_PREFIX + "jobs/"

	DEFAULT_TEST_MODE_VALUE = false
)

type CLIFlags struct {
	ConfigPath string
	RootCert   string
	Localmode  bool
}

// Pattern one global app struct that contains all services
type App struct {
	// A global wait group, all go routines that should
	// terminate when the application ends should be registered here
	WG sync.WaitGroup

	ReloadSignal chan os.Signal
	ExitSignal   chan os.Signal

	// The CLIFlags passed to the application
	CliFlags *CLIFlags
	Config   *config.Config

	DbusClient *bus.DbusClient

	OtaService     rauc.RaucService
	GpsService     gps.GPSService
	NetworkService net.NetworkService
}

func (a *App) Shutdown() {
	if a.GpsService != nil {
		a.GpsService.Shutdown()
	}

	if a.OtaService != nil {
		a.OtaService.Shutdown()
	}

	if a.NetworkService != nil {
		a.NetworkService.Shutdown()
	}

	// Close the (d)-bus client as the last thing
	if a.DbusClient != nil {
		a.DbusClient.Shutdown()
	}
}

func (a *App) SensorName() string {
	return a.Config.Client.Authentication.SensorName
}

func ParseCLIFlags() *CLIFlags {
	flags := &CLIFlags{}
	flag.StringVar(&flags.ConfigPath, "config", DEFAULT_CONFIG_PATH, "relative or absolute path to the config file")
	flag.StringVar(&flags.RootCert, "rootcert", DEFAULT_ROOT_CERT, "relative or absolute path to the root certificate used for server validation")
	flag.BoolVar(&flags.Localmode, "local", DEFAULT_TEST_MODE_VALUE, "true if the local (no api connections) mode should be used for testing")
	flag.Parse()

	return flags
}

func setDefaults(config *config.Config, flags *CLIFlags) (*config.Config, error) {
	// If the cert specified on the cli is not the default one, use it instead
	if flags.RootCert != DEFAULT_ROOT_CERT {
		config.Client.RootCert = &flags.RootCert
	}

	// Set up the default directories
	if config.Client.Jobs.TempCollectStorage == "" {
		config.Client.Jobs.TempRecStorage = DEFAULT_JOB_COLLECT_DIR
	}

	if config.Client.Jobs.TempRecStorage == "" {
		config.Client.Jobs.TempRecStorage = DEFAULT_JOB_TMP_DIR
	}

	return config, nil
}

func loadConfiguration(app *App) {
	flags := app.CliFlags
	configPath := flags.ConfigPath

	var err error

	// Check given configFile
	if err = config.ValidatePath(configPath); err != nil {
		apglog.Error("error while loading configuration: " + err.Error())

		// Fallback to default
		configPath = DEFAULT_CONFIG_PATH
		if err = config.ValidatePath(configPath); err != nil {
			apglog.Fatal("all possible configuration paths exhausted, error while loading: " + err.Error())
		}
	}

	// Decode config file, no field validation is taking place here, the code using it is required to check for required fields etc.
	app.Config, err = config.NewConfiguration(configPath)
	if err != nil {
		apglog.Error("an error occurred while trying to load the config file " + configPath + ", error: " + err.Error())
		return
	}

	// Set the defaults in case the user omitted some fields
	if _, err = setDefaults(app.Config, app.CliFlags); err != nil {
		apglog.Error("How could this error happen? " + err.Error())
	}

	// Check given certFile
	if err := config.ValidatePath(*app.Config.Client.RootCert); err != nil {
		apglog.Error("error while loading certificate: " + err.Error())

		// Fallback to default, if the previous one was already the default we might have a problem here
		flags.RootCert = DEFAULT_ROOT_CERT
		if err = config.ValidatePath(flags.RootCert); err != nil {
			apglog.Error("all possible certificate paths exhausted, error while loading: " + err.Error())
		}
	}

	apglog.Debug("Active configuration", zap.Any("config", *app.Config))
}

// todo: error handling
func connectToSystemDBUS(app *App) error {
	// Bring up the dbus connections
	dbusClient := bus.NewDbusClient()
	dbusClient.Connect()

	app.DbusClient = dbusClient

	return nil
}

func startGPSService(app *App) {
	// Start GPSD Monitor, fall back to stub if a startup failure happened
	gpsService, err := gps.NewService(gps.GPSD, &gps.GpsdBackendParameters{Conn: app.DbusClient.GetConnection()})
	if err != nil {
		apglog.Error("Could not initialize gpsd data backend, falling back to stub", zap.Error(err))
		gpsService, _ = gps.NewService(gps.STUB, nil)
	}

	apglog.Info("Location received", zap.String("data", gpsService.GetData().String()))
	app.GpsService = gpsService
}

// Sets up the updater service
// This only supports RAUC for now but i
func setupOTAService(app *App) {
	otaService, err := rauc.NewService(app.DbusClient.GetConnection())
	if err != nil {
		apglog.Error("OTA Service could not be initialized")
		// todo: implement stub for platforms without OTA support
	}

	app.OtaService = otaService
}

func startNetworkService(app *App) {
	nsvc, err := net.NewService(app.DbusClient.GetConnection())
	if err != nil {
		apglog.Error("Network service could not be started")
	}

	app.NetworkService = nsvc
}
func setupAPI(app *App) error {
	cc := app.Config.Client
	cProv := cc.Provisioning

	// Set up the REST api
	apiBaseURL := "https://" + cProv.Host + ":" + cProv.Port + cProv.Path
	api.SetupAPI(apiBaseURL, cc.RootCert, cc.Authentication.SensorName, cc.Authentication.Password)

	// todo: error handling
	return nil
}

func Setup() (*App, error) {
	app := App{}
	app.CliFlags = ParseCLIFlags()

	// Register a quit signal
	app.ExitSignal = make(chan os.Signal, 1)
	signal.Notify(app.ExitSignal, os.Interrupt, syscall.SIGTERM)

	// Register the reload signal
	app.ReloadSignal = make(chan os.Signal, 1)
	signal.Notify(app.ReloadSignal, syscall.SIGUSR1, syscall.SIGUSR2)

	// Initialize logger
	apglog.Init()
	apglog.Info("apogee-client starting")

	// Load the configuration file
	loadConfiguration(&app)

	// Connect to SystemDBUS
	err := connectToSystemDBUS(&app)

	if err == nil {
		// Start GPSService
		startGPSService(&app)
		// Prepare OTAService
		setupOTAService(&app)
		// Prepare network Service
		startNetworkService(&app)
	} else {
		apglog.Fatal("Could not initialize system dbus connection and required services", zap.Error(err))
	}

	// Set up the remote API
	err = setupAPI(&app)

	// If api setup fails and we are not in local mode, terminate application
	if err != nil {
		return &app, err
	}

	return &app, nil
}
