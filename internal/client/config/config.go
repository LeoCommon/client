package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"github.com/BurntSushi/toml"
	"go.uber.org/zap"
)

type Config struct {
	Api  api.Config
	Jobs struct {
		StoragePath     string
		TempPath        string
		PollingInterval time.Duration
		Iridium         struct{ Disabled bool }
		Network         struct{ Disabled bool }
	}
}

func NewConfiguration(path string) (*Config, error) {
	config := &Config{}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Error("Failed to read config file", zap.Error(err))
	}

	if err = toml.Unmarshal(data, &config); err != nil {
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
