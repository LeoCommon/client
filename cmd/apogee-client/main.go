package main

import (
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/config"
	"disco.cs.uni-kl.de/apogee/pkg/jobHandler"
	dbusclient "disco.cs.uni-kl.de/apogee/pkg/system/dbusclient"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/gps"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/rauc"
	"go.uber.org/zap"
)

var (
	PRODUCT_NAME              = "apogee"
	USERDATA_DIRECTORY_PREFIX = "/data/"
	CONFIG_FOLDER             = "discosat-config/"

	CONFIG_PATH_PREFIX = CONFIG_FOLDER + PRODUCT_NAME + "/"
	CONFIG_FILE        = "config.yml"
	CERT_FILE          = "discosat.crt"

	DEGAULT_CONFIG_PATH = USERDATA_DIRECTORY_PREFIX + CONFIG_PATH_PREFIX + CONFIG_FILE
	DEFAULT_ROOT_CERT   = USERDATA_DIRECTORY_PREFIX + CONFIG_PATH_PREFIX + CERT_FILE
)

type CLIFlags struct {
	configPath string
	rootCert   string
}

// Pattern one global app struct that contains all services
type AppStruct struct {
	otaService rauc.RaucService
	rootCert   string
}

func ParseCLIFlags() *CLIFlags {
	flags := &CLIFlags{}
	flag.StringVar(&flags.configPath, "config", DEGAULT_CONFIG_PATH, "relative or absolute path to the config file")
	flag.StringVar(&flags.rootCert, "rootcert", DEFAULT_ROOT_CERT, "relative or absolute path to the root certificate used for server validation")
	flag.Parse()

	return flags
}

func setDefaults(config *config.Config, flags *CLIFlags) (*config.Config, error) {
	// If no rootCert given in the config, use the default root certificate path
	if config.Client.RootCert == nil {
		config.Client.RootCert = &flags.rootCert
	}

	return config, nil
}

func main() {
	var wg sync.WaitGroup

	// Initialize logger
	apglog.Init()

	apglog.Info("apogee-apogee-client starts")
	flags := ParseCLIFlags()

	configPath := flags.configPath

	// Check given configFile
	if err := config.ValidatePath(configPath); err != nil {
		apglog.Error("error while loading configuration: " + err.Error())

		// Fallback to default
		configPath = DEGAULT_CONFIG_PATH
		if err = config.ValidatePath(configPath); err != nil {
			apglog.Fatal("all possible configuration paths exhausted, error while loading: " + err.Error())
		}
	}

	// Decode config file, no field validation is taking place here, the code using it is required to check for required fields etc.
	systemConfig, err := config.NewConfiguration(configPath)
	if err != nil {
		apglog.Error("an error occurred while trying to load the config file " + configPath + ", error: " + err.Error())
		return
	}

	// Set the defaults in case the user omitted some fields
	if _, err = setDefaults(systemConfig, flags); err != nil {
		apglog.Error("How could this error happen? " + err.Error())
	}

	// Check given certFile
	if err := config.ValidatePath(flags.rootCert); err != nil {
		apglog.Error("error while loading certificate: " + err.Error())

		// Fallback to default
		flags.rootCert = DEFAULT_ROOT_CERT
		if err = config.ValidatePath(flags.rootCert); err != nil {
			apglog.Error("all possible certificate paths exhausted, error while loading: " + err.Error())
		}
	}

	// Bring up the dbus connections
	dbusClient := dbusclient.NewDbusClient()
	dbusClient.Connect()
	defer dbusClient.Close()

	// Initialize the rauc service connection
	raucService, _ := rauc.NewService(dbusClient.GetConnection())

	// fixme: remove debug code
	slot, err := raucService.MarkBooted(rauc.SLOT_STATUS_GOOD)
	if err != nil {
		apglog.Error("RAUC marking failed with", zap.String("slot", slot), zap.Error(err))
	}

	// Start GPSD Monitor, fall back to stub if a startup failure happened
	gpsService, err := gps.NewService(gps.GPSD, &gps.GpsdBackendParameters{Conn: dbusClient.GetConnection()})
	if err != nil {
		apglog.Error("Could not initialize gpsd data backend, falling back to stub", zap.Error(err))
		gpsService, _ = gps.NewService(gps.STUB, nil)
	}

	apglog.Info("Location received", zap.String("data", gpsService.GetData().String()))

	/*
		ticker := time.NewTicker(5 * time.Second)
		quit := make(chan struct{})
		go func() {
			for {
				select {
				case <-ticker.C:
					apglog.Debug(gpsService.GetData().String())
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()
	*/

	cc := systemConfig.Client
	cProv := cc.Provisioning

	// Set up the serverAPI
	apiBaseURL := "https://" + cProv.Host + ":" + cProv.Port + cProv.Path
	api.SetupAPI(apiBaseURL, *cc.RootCert, cc.Authentication.SensorName, cc.Authentication.Password)

	// Use this status collection and upload as a check for system-functionality. If everything works set system okay.
	myStatus, err := jobHandler.GetDefaultSensorStatus()
	if err != nil {
		apglog.Error("unable get a clean default sensor status: " + err.Error())
		err := api.PutSensorUpdate(myStatus)
		if err != nil {
			apglog.Error("unable to put initial unclean sensor update on server: " + err.Error())
		}
	} else {
		err := api.PutSensorUpdate(myStatus)
		if err != nil {
			apglog.Error("unable to put initial clean sensor update on server: " + err.Error())
		} else {
			// If the default-status was clean and the status-push was clean, the core should be functional
			apglog.Debug("Daemon startup successful, marking slot as good")
			slot, err := raucService.MarkBooted(rauc.SLOT_STATUS_GOOD)
			if err != nil {
				apglog.Fatal("RAUC marking failed with", zap.String("slot", slot), zap.Error(err))
			}
		}
	}

	apglog.Info("Loading config done. Starting main loop...")

	quitSignal := make(chan os.Signal, 1)
	signal.Notify(quitSignal, os.Interrupt, syscall.SIGTERM)

	jobTicker := time.NewTicker(time.Duration(cc.PollingInterval) * time.Second)

	wg.Add(1)

	// Attention: "tick shifts"
	// If the execution takes more time, consequent runs are delayed
	go func() {
		for {
			select {
			case <-jobTicker.C:
				// Tell the server you are alive
				myStatus, _ := jobHandler.GetDefaultSensorStatus()
				err := api.PutSensorUpdate(myStatus)
				if err != nil {
					apglog.Error("unable to put sensor update on server: " + err.Error())
				}

				// Pull jobs and schedule the execution
				apglog.Debug("Polling jobs.")
				myJobs, err := api.GetJobs()
				if err != nil {
					apglog.Error("unable to pull jobs from server: " + err.Error())
					jobHandler.HandleOldJobs(cc.PollingInterval)
				} else {
					jobHandler.HandleNewJobs(myJobs, cc.PollingInterval, cc.Authentication.SensorName)
				}
				// do stuff
			case <-quitSignal:
				jobTicker.Stop()
				wg.Done()
				return
			}
		}
	}()

	// Wait until everything terminates
	wg.Wait()

	apglog.Info("apogee shutting down")

	// Shutdown logic
	gpsService.Shutdown()
}
