package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Client struct {
		Provisioning struct {
			Host string `yaml:"host"`
			Port string `yaml:"port"`
			Path string `yaml:"path"`
		} `yaml:"provisioning"`
		RootCert       *string `yaml:"root_certificate,omitempty"`
		Authentication struct {
			SensorName string `yaml:"sensor_name"`
			Password   string `yaml:"password"`
		} `yaml:"authentication"`
		PollingInterval int64 `yaml:"polling_interval"`
		Jobs            struct {
			StoragePath string `yaml:"storage_path"`
			TempPath    string `yaml:"temp_path"`
		} `yaml:"jobs"`
	} `yaml:"apogee"`
}

func NewConfiguration(path string) (*Config, error) {
	config := &Config{}

	// Try to get the existing config from the supplied path
	cFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	// Close the file later
	defer cFile.Close()

	// Decode the config
	y := yaml.NewDecoder(cFile)
	if err := y.Decode(&config); err != nil {
		return nil, err
	}

	return config, nil
}

func ValidatePath(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}

	if s.IsDir() {
		return fmt.Errorf("supplied config file '%s' is a directory", path)
	}

	return nil
}
