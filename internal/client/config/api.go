package config

import (
	"sync"

	"go.uber.org/atomic"
)

// Config contains the api configuration options
type API struct {
	SensorName string `toml:"sensorname"`
	Url        string `toml:"url"`

	Security struct {
		Basic *struct {
			Username string `toml:"username,omitempty"`
			Password string `toml:"password"`
		} `toml:"basic,omitempty"`

		Bearer *struct {
			RefreshToken atomic.String `toml:"refresh_token"`
		} `toml:"bearer,omitempty"`

		RootCertificate atomic.String `toml:"root_certificate,omitempty"`
		AllowInsecure   bool          `toml:"allow_insecure,omitempty"`
	} `toml:"security"`
}

type ApiConfig struct {
	mu   sync.RWMutex
	conf *API
}

func (a *ApiConfig) SensorName() string {
	return a.conf.SensorName
}

func (a *ApiConfig) Url() string {
	return a.conf.Url
}

func (a *ApiConfig) AllowInsecure() bool {
	return a.conf.Security.AllowInsecure
}

func (c *ApiConfig) BasicAuth() (string, string) {
	if c.conf.Security.Basic == nil {
		return "", ""
	}

	// Handle missing username
	username := c.conf.Security.Basic.Username
	if len(username) == 0 {
		username = c.conf.SensorName
	}

	return username, c.conf.Security.Basic.Password
}

func (c *ApiConfig) HasBasicAuth() bool {
	return c.conf.Security.Basic != nil && len(c.conf.Security.Basic.Password) > 0
}

// Concurrency safe methods below

func (a *ApiConfig) SetRefreshToken(token string) {
	a.conf.Security.Bearer.RefreshToken.Store(token)
}

func (a *ApiConfig) RefreshToken() string {
	if a.conf.Security.Bearer == nil {
		return ""
	}

	return a.conf.Security.Bearer.RefreshToken.Load()
}

func (a *ApiConfig) SetRootCertificate(cert string) {
	a.conf.Security.RootCertificate.Store(cert)
}

func (a *ApiConfig) RootCertificate() string {
	return a.conf.Security.RootCertificate.Load()
}
