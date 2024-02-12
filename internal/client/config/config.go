package config

import (
	"flag"
	"os"
	"sync"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"github.com/pelletier/go-toml/v2"
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

type MainConfig struct {
	Client ClientConfig `toml:"client"`
	Api    ApiConfig    `toml:"api,omitempty"`
	Jobs   JobsConfig   `toml:"jobs,omitempty"`
}

type ConfigManager interface {
	lock()
	unlock()
	Verify() error
}

type ConfigManagerKey string

const (
	CMClient ConfigManagerKey = "client"
	CMApi    ConfigManagerKey = "api"
	CMJob    ConfigManagerKey = "job"
)

type ConfigManagerStore map[ConfigManagerKey]ConfigManager

type Manager struct {
	mu sync.RWMutex

	// The actual config, never share this with other code
	config *MainConfig
	flags  *CLIFlags

	// The config manager store (pointers)
	store ConfigManagerStore

	// The config path
	path string
}

func (m *Manager) Client() *ClientConfigManager {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cm, ok := m.store[CMClient].(*ClientConfigManager)
	if !ok {
		log.Panic("implementation mistake, no CMJob found")
		return nil
	}
	return cm
}

func (m *Manager) Api() *ApiConfigManager {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cm, ok := m.store[CMApi].(*ApiConfigManager)
	if !ok {
		log.Panic("implementation mistake, no CMApi found")
		return nil
	}
	return cm
}

func (m *Manager) Job() *JobConfigManager {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cm, ok := m.store[CMJob].(*JobConfigManager)
	if !ok {
		log.Panic("implementation mistake, no CMJob found")
		return nil
	}
	return cm
}

func (m *Manager) Load(path string, acceptEmptyConfig bool) error {
	data, err := os.ReadFile(path)
	if err == nil {
		if err = toml.Unmarshal(data, m.config); err != nil {
			log.Error("failed to unmarshal config file", zap.Error(err))
		}
	}

	if err != nil && !acceptEmptyConfig {
		return err
	}

	// Store the load path
	m.path = path

	// Each config section manager gets his own locking primitive
	m.store = ConfigManagerStore{
		// Save the general client section
		CMClient: NewClientConfigManager(&m.config.Client, m),
		// API Section
		CMApi: NewApiConfigManager(&m.config.Api, m),
		// Job Section
		CMJob: NewJobConfigManager(&m.config.Jobs, m),
	}

	// Verify all configs contain the mandatory values
	for _, value := range m.store {
		if err := value.Verify(); err != nil {
			return err
		}
	}

	// Debug log output
	log.Debug("active config", zap.Any("config", m.config), zap.String("path", m.path))

	return nil
}

// Save locks all configs and writes it to disk
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Lock all config managers
	for _, value := range m.store {
		value.lock()
	}

	// Unlock the config managers when we are done
	defer func() {
		for _, value := range m.store {
			value.unlock()
		}
	}()

	// Marshal the config, does not use getters, so no locking => safe
	configData, err := toml.Marshal(m.config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(m.path, configData, 0644); err != nil {
		log.Error("Failed to write config file", zap.Error(err))
		return err
	}

	return nil
}

func New() *MainConfig {
	return &MainConfig{}
}

func NewManager() *Manager {
	return &Manager{
		mu:     sync.RWMutex{},
		store:  make(ConfigManagerStore),
		config: New(),
	}
}

var once sync.Once

func ParseCLIFlags() CLIFlags {
	flags := CLIFlags{}

	flag.StringVar(&flags.ConfigPath, "config", DefaultConfigPath, "relative or absolute path to the config file")
	flag.StringVar(&flags.RootCert, "rootcert", "", "relative or absolute path to the root certificate used for server validation")
	flag.BoolVar(&flags.Debug, "debug", DefaultDebugModeValue, "true if the debug logging should be enabled")

	flag.Parse()

	return flags
}

type TOMLDuration time.Duration

func (d *TOMLDuration) UnmarshalText(b []byte) error {
	x, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = TOMLDuration(x)
	return nil
}

func (c TOMLDuration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(c).String()), nil
}

func (c *TOMLDuration) Value() time.Duration {
	return time.Duration(*c)
}

// Convenience methods for certain things like SensorName
// todo: make them go away when re-writing everything that depends on these
func (m *Manager) SensorName() string {
	// Grab the client config manager instance
	return m.Client().C().SensorName
}

func (m *Manager) JobTempPath() string {
	return m.Job().C().TempDir.String()
}

func (m *Manager) JobStoragePath() string {
	return m.Job().C().StorageDir.String()
}

func (m *Manager) SetJobTempPath(newJobTempPath string) error {
	m.Job().Set(func(config *JobsConfig) {
		config.TempDir = TempPath(newJobTempPath)
	})
	m.Job().Save()
	return nil
}

func (m *Manager) SetJobStoragePath(newJobStoragePath string) error {
	m.Job().Set(func(config *JobsConfig) {
		config.StorageDir = StoragePath(newJobStoragePath)
	})
	m.Job().Save()
	return nil
}

func (m *Manager) SetPollingInterval(newPollingInterval string) error {
	duration, err := time.ParseDuration(newPollingInterval)
	if err != nil {
		return err
	}
	m.Job().Set(func(config *JobsConfig) {
		config.PollingInterval = TOMLDuration(duration)
	})
	m.Job().Save()
	return nil
}
