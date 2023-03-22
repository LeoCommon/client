package config

import (
	"flag"
	"os"
	"regexp"
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
	DefaultPollingInterval = ShortDuration(time.Second * 30)

	DefaultDebugModeValue = false
)

type CLIFlags struct {
	ConfigPath string
	RootCert   string
	Debug      bool
}

type config struct {
	Api  API  `toml:"api,omitempty"`
	Jobs Jobs `toml:"jobs,omitempty"`
}

type Manager struct {
	mu     sync.RWMutex
	config *config
	flags  *CLIFlags

	// Store the config "objects" here
	mutJob *JobConfig
	mutApi *ApiConfig

	// The config path
	path string
}

func (m *Manager) Api() *ApiConfig {
	return m.mutApi
}

func (m *Manager) Jobs() *JobConfig {
	return m.mutJob
}

func (m *Manager) setDefaults() {
	// Set up the default directories
	if m.config.Jobs.StoragePath.Load() == "" {
		m.config.Jobs.StoragePath.Store(DefaultJobStorageDir)
	}

	if m.config.Jobs.TempPath.Load() == "" {
		m.config.Jobs.TempPath.Store(DefaultJobTmpDir)
	}

	// This prevents zero polling intervals
	if m.config.Jobs.PollingInterval == 0 {
		m.config.Jobs.PollingInterval = DefaultPollingInterval
	}
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

	// Fallback to defaults
	m.setDefaults()

	// Store the load path
	m.path = path

	// Create the mutators
	m.mutApi = &ApiConfig{
		conf: &m.config.Api,
		mu:   sync.RWMutex{},
	}
	m.mutJob = &JobConfig{
		conf: &m.config.Jobs,
		mu:   sync.RWMutex{},
	}

	// Debug log output
	log.Debug("active config", zap.Any("config", m.config), zap.String("path", m.path))

	return nil
}

func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

func New() *config {
	return &config{}
}

// todo: rework, make writes concurrency safe,
func NewManager() *Manager {
	return &Manager{
		mu:     sync.RWMutex{},
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

var ShortDurationMatcher = regexp.MustCompile(`([0-9]+h)?([0-9]+m)?([0-9]+s)?`)

type ShortDuration time.Duration

func (d *ShortDuration) UnmarshalText(b []byte) error {
	x, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = ShortDuration(x)
	return nil
}

func (c ShortDuration) MarshalText() ([]byte, error) {
	matches := ShortDurationMatcher.FindStringSubmatch(time.Duration(c).String())
	shortDuration := matches[1] + matches[2] + matches[3]

	return []byte(shortDuration), nil
}

func (c *ShortDuration) Value() time.Duration {
	return time.Duration(*c)
}
