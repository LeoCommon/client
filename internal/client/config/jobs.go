package config

import (
	"sync"
	"time"

	"go.uber.org/atomic"
)

// These are basic settings for every job
type BaseJobSettings struct {
	Disabled bool `toml:"disabled"`
}

// If you want to modify any field at run-time here, make sure to lock it using a mutex
type Jobs struct {
	StoragePath     atomic.String   `toml:"storage_path,omitempty"`
	TempPath        atomic.String   `toml:"temp_path,omitempty"`
	PollingInterval ShortDuration   `toml:"polling_interval,omitempty"`
	Iridium         BaseJobSettings `toml:"iridium"`
	Network         BaseJobSettings `toml:"network"`
}

type JobConfig struct {
	mu   sync.RWMutex
	conf *Jobs
}

func (j *JobConfig) SetStoragePath(path string) {
	j.conf.StoragePath.Store(path)
}

func (j *JobConfig) StoragePath() string {
	return j.conf.StoragePath.Load()
}

func (j *JobConfig) SetTempPath(path string) {
	j.conf.TempPath.Store(path)
}

func (j *JobConfig) TempPath() string {
	return j.conf.TempPath.Load()
}

func (j *JobConfig) SetPollingInterval(dur time.Duration) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.conf.PollingInterval = ShortDuration(dur)
}

func (j *JobConfig) PollingInterval() time.Duration {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return time.Duration(j.conf.PollingInterval)
}

func (j *JobConfig) SetIridiumEnabled(state bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.conf.Iridium.Disabled = !state
}

func (j *JobConfig) IsIridiumDisabled() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.conf.Iridium.Disabled
}

func (j *JobConfig) SetNetworkEnabled(state bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.conf.Network.Disabled = !state
}

func (j *JobConfig) IsNetworkDisabled() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.conf.Network.Disabled
}
