package config

// These are basic settings for every job
type BaseJobSettings struct {
	Disabled bool `toml:"disabled,omitempty"`
}

type StoragePath string

func (j StoragePath) String() string {
	if j == "" {
		return DefaultJobStorageDir
	}

	return string(j)
}

type TempPath string

func (j TempPath) String() string {
	if j == "" {
		return DefaultJobTmpDir
	}

	return string(j)
}

type JobsConfig struct {
	StorageDir      StoragePath     `toml:"storage_path,omitempty"`
	TempDir         TempPath        `toml:"temp_path,omitempty"`
	PollingInterval TOMLDuration    `toml:"polling_interval,omitempty"`
	Iridium         BaseJobSettings `toml:"iridium"`
	Network         BaseJobSettings `toml:"network"`
}

type JobConfigManager struct {
	BaseConfigManager[JobsConfig]
}

// Verify verifies the "hard" conditions that the rest of the code relies on
func (a *JobConfigManager) Verify() error {
	return nil
}

func NewJobConfigManager(config *JobsConfig, mgr *Manager) *JobConfigManager {
	j := JobConfigManager{}
	j.conf = config
	j.mgr = mgr

	return &j
}
