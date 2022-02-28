package main

import (
	"flag"
	"fmt"
	"log"
	"math/big"

	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/config"
	"disco.cs.uni-kl.de/apogee/pkg/crypto"
)

var (
	PRODUCT_NAME              = "apogee"
	CONFIG_DIRECTORY_PREFIX   = "/etc/"
	USERDATA_DIRECTORY_PREFIX = "/data/"

	CONFIG_PATH_PREFIX = PRODUCT_NAME + "/"
	CONFIG_FILE        = "config.yml"
	CONFIG_FILE_PATH   = CONFIG_PATH_PREFIX + CONFIG_FILE

	DEFAULT_ROOT_CERT = CONFIG_DIRECTORY_PREFIX + CONFIG_PATH_PREFIX + "root.pem"
)

type CLIFlags struct {
	configPath string
	rootCert   string
}

func ParseCLIFlags() *CLIFlags {
	flags := &CLIFlags{}
	flag.StringVar(&flags.configPath, "config", CONFIG_DIRECTORY_PREFIX+CONFIG_FILE_PATH, "relative or absolute path to the config file")
	flag.StringVar(&flags.rootCert, "rootcert", DEFAULT_ROOT_CERT, "relative or absolute path to the root certificate used for server validation")
	flag.Parse()

	return flags
}

func setDefaults(config *config.Config, flags *CLIFlags) (*config.Config, error) {
	// Set default root certificate path
	if config.Client.RootCert == nil {
		config.Client.RootCert = &flags.rootCert
	}

	return config, nil
}

func main() {
	flags := ParseCLIFlags()

	configPath := flags.configPath

	// Try to load burned in config, this is possible to be overriden by the user
	if err := config.ValidatePath(configPath); err != nil {
		log.Println(fmt.Errorf("error while loading configuration: %w", err))

		// Fallback to user directory
		configPath = USERDATA_DIRECTORY_PREFIX + CONFIG_FILE_PATH
		if err = config.ValidatePath(configPath); err != nil {
			log.Fatalf("all possible configuration paths exhuasted, error while loading: %v", err)
		}
	}

	// Decode config file, no field validation is taking place here, the code using it is required to check for required fields etc.
	systemConfig, err := config.NewConfiguration(configPath)
	if err != nil {
		fmt.Println(fmt.Errorf("an error occured while trying to load the config file '%v', error: '%w'", configPath, err))
		return
	}

	// Set the defaults in case the user omitted some fields
	setDefaults(systemConfig, flags)

	cc := systemConfig.Client
	cProv := cc.Provisioning

	apiBaseURL := "https://" + cProv.Host + ":" + cProv.Port + cProv.Path
	apiClient := api.SetupAPI(apiBaseURL, *cc.RootCert)

	apiClient.IsAdopted()

	fmt.Printf("\nConfig %v\n", systemConfig)

	// Load temporary self signed client certificate with id 0
	cert, err := crypto.CreateX509KeyPair(big.NewInt(0))
	if err != nil {
		log.Fatalf("Error while trying to create client certificate %v", err)
	}

	apiClient.LoadClientCertificate(*cert)

}
