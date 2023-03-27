package config

import (
	"errors"
	"net/url"

	jwtmisc "disco.cs.uni-kl.de/apogee/internal/client/api/jwt/misc"
	"golang.org/x/exp/slices"
)

// The bearer token source
type BearerTokenSource string

const (
	// Keep these names synced up with the toml AuthBearerSettings below
	BearerSourceBody    BearerTokenSource = "body"
	BearerSourceCookies BearerTokenSource = "cookies"
)

// SupportedOptions lists the options for the config parser
func (b BearerTokenSource) SupportedOptions() []BearerTokenSource {
	return []BearerTokenSource{
		BearerSourceBody,
		BearerSourceCookies,
	}
}

type BearerCookieSettings struct {
	RefreshTokenName string `toml:"refresh_name,omitempty" comment:"name of the refresh token cookie sent from the server"`
	AccessTokenName  string `toml:"access_name,omitempty" comment:"name of the access token cookie sent from the server"`
}

type BearerHeaderSettings struct {
	Scheme string `toml:"scheme,omitempty" comment:"changes <Scheme>, results in 'Authorization: <Scheme> <Token>', defaults to Bearer"`
}

type AuthBearerSettings struct {
	Sources *[]BearerTokenSource `toml:"sources,omitempty" comment:"specifies enabled token sources."`
	jwtmisc.TokenPair
	CookieSettings  BearerCookieSettings `toml:"cookies,omitempty" comment:"cookie source specific settings"`
	RefreshEndpoint string               `toml:"refresh_endpoint,omitempty" comment:"custom relative url to the bearer refresh endpoint"`
	HeaderSettings  BearerHeaderSettings `toml:"header,omitempty" comment:"authorization header specific settings"`
}

func (a *AuthBearerSettings) BodySourceEnabled() bool {
	return a.Sources != nil && slices.Contains(*a.Sources, BearerSourceBody)
}

func (a *AuthBearerSettings) CookieSourceEnabled() bool {
	return a.Sources != nil && slices.Contains(*a.Sources, BearerSourceCookies)
}

type AuthBasicSettings struct {
	Username string `toml:"username,omitempty"`
	Password string `toml:"password" comment:"required for basic authentication"`
}

func (a *AuthBasicSettings) Credentials() (string, string) {
	return a.Username, a.Password
}

type AuthSettings struct {
	Basic  *AuthBasicSettings  `toml:"basic,omitempty"`
	Bearer *AuthBearerSettings `toml:"bearer,omitempty" comment:"Bearer authentication settings"`
}

// Config contains the api configuration options
type ApiConfig struct {
	Auth            AuthSettings `toml:"auth"`
	RootCertificate string       `toml:"root_certificate,omitempty"`
	Url             string       `toml:"url"`
	AllowInsecure   bool         `toml:"allow_insecure,omitempty"`
}

type ApiConfigManager struct {
	BaseConfigManager[ApiConfig]
}

// Verify verifies the "hard" conditions that the rest of the code relies on
func (a *ApiConfigManager) Verify() error {
	// Verify the url
	if _, err := url.Parse(a.conf.Url); err != nil {
		return err
	}

	// Verify that auth basic contains a password
	if a.conf.Auth.Basic != nil && a.conf.Auth.Basic.Password == "" {
		return errors.New("empty password for auth basic")
	}

	// If bearer auth is enabled:
	bearer := a.conf.Auth.Bearer
	if bearer != nil {
		// we at-least need a refresh token
		if bearer.Refresh == "" {
			return errors.New("bearer auth enabled but no refresh token specified")
		}

		// and if the source slice is populated it should not be empty
		if bearer.Sources != nil &&
			!bearer.BodySourceEnabled() && !bearer.CookieSourceEnabled() {
			return errors.New("manually disabling all bearer token sources is forbidden")
		}
	}

	return nil
}

func NewApiConfigManager(config *ApiConfig, mgr *Manager) *ApiConfigManager {
	j := ApiConfigManager{}
	j.conf = config
	j.mgr = mgr

	return &j
}
