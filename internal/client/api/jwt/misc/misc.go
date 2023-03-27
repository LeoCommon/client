package misc

import (
	"errors"
	"time"
)

const (
	DefaultRefreshTokenCookieName = "refresh_token_cookie"
	DefaultAuthTokenCookieName    = "access_token_cookie"
	DefaultSchemeName             = "Bearer"
	DefaultRefreshEndpoint        = "login/refresh"

	// This expiry offset makes sure we refresh tokens ahead of time even when there is clock skew
	ExpiryOffset = 5 * time.Second
)

var (
	ErrInvalidJWTSettings  = errors.New("invalid JWT settings supplied")
	ErrRefreshTokenInvalid = errors.New("the refresh token is invalid")
	ErrTokenMissing        = errors.New("empty/missing token")
)

type TokenPair struct {
	Refresh string `json:"refresh_token" toml:"refresh_token,omitempty" comment:"required refresh token"`
	Access  string `json:"access_token"  toml:"access_token,omitempty" comment:"optional access token"`
}

func (p *TokenPair) HasRefreshToken() bool {
	return len(p.Refresh) > 0
}

func (p *TokenPair) HasAuthToken() bool {
	return len(p.Access) > 0
}

func (p *TokenPair) FullTokenPair() bool {
	return p.HasRefreshToken() && p.HasAuthToken()
}
