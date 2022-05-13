package main

import (
	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/config"
	"disco.cs.uni-kl.de/apogee/pkg/jobHandler"
	"flag"
	"time"
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
	apglog.Debug("apogee-apogee-client starts")
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

	// Check given certFile
	if err := config.ValidatePath(flags.rootCert); err != nil {
		apglog.Error("error while loading certificate: " + err.Error())

		// Fallback to default
		flags.rootCert = DEFAULT_ROOT_CERT
		if err = config.ValidatePath(flags.rootCert); err != nil {
			apglog.Fatal("all possible certificate paths exhausted, error while loading: " + err.Error())
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

	cc := systemConfig.Client
	cProv := cc.Provisioning

	// Set up the serverAPI
	apiBaseURL := "https://" + cProv.Host + ":" + cProv.Port + cProv.Path
	api.SetupAPI(apiBaseURL, *cc.RootCert, cc.Authentication.SensorName, cc.Authentication.Password)

	apglog.Debug("Loading config done. Starting main loop...")
	// The main loop
	for {
		// Tell the server you are alive
		myStatus := jobHandler.GetDefaultSensorStatus()
		if err := api.PutSensorUpdate(myStatus); err != nil {
			apglog.Error("unable to put sensor update on server: " + err.Error())
		}

		// Pull jobs and schedule the execution
		apglog.Debug("Polling jobs.")
		myJobs, err := api.GetJobs()
		if err != nil {
			apglog.Error("unable to pull jobs from server: " + err.Error())
			jobHandler.HandleOldJobs()
		} else {
			jobHandler.HandleNewJobs(myJobs)
		}

		// Wait until next pull
		time.Sleep(time.Duration(cc.PollingInterval) * time.Second)
	}
}
