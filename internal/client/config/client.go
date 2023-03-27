package config

// If you want to modify any field at run-time here, make sure to lock it using a mutex
type ClientConfig struct {
	SensorName string `toml:"sensor_name,omitempty"`
	Debug      bool   `toml:"debug"`
}

type ClientConfigManager struct {
	BaseConfigManager[ClientConfig]
}

// Verify verifies the "hard" conditions that the rest of the code relies on
func (a *ClientConfigManager) Verify() error {
	return nil
}

func NewClientConfigManager(config *ClientConfig, mgr *Manager) *ClientConfigManager {
	j := ClientConfigManager{}
	j.conf = config
	j.mgr = mgr

	return &j
}
