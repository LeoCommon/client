package jwt

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	gojwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

const (
	RefreshTokenCookieName = "refresh_token_cookie"
	AuthTokenCookieName    = "access_token_cookie"

	// This expiry offset makes sure we refresh tokens ahead of time even when there is clock skew
	ExpiryOffset = 5 * time.Second
)

var ErrTokenMissing = errors.New("empty/missing token")

type TokenReply struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type TokenPair struct {
	Refresh string
	Auth    string
}

func (p *TokenPair) HasRefreshToken() bool {
	return len(p.Refresh) > 0
}

func (p *TokenPair) HasAuthToken() bool {
	return len(p.Auth) > 0
}

func (p *TokenPair) Empty() bool {
	return !p.HasAuthToken() && !p.HasRefreshToken()
}

func (p *TokenPair) FullPair() bool {
	return p.HasRefreshToken() && p.HasAuthToken()
}

func Validate(tokenString string) error {
	// token is empty, this happens on restarts when only the refresh token is available
	if len(tokenString) == 0 {
		return ErrTokenMissing
	}

	parser := gojwt.NewParser()

	// Try parsing in MapClaims mode
	token, _, err := parser.ParseUnverified(tokenString, gojwt.MapClaims{})

	if err != nil {
		// Try the registered claims mode
		token, _, err = parser.ParseUnverified(tokenString, &gojwt.RegisteredClaims{})

		// This shadows the above error, but thats fine as this is an implementation problem
		if err != nil {
			log.Error("both JWT parsing methods failed, check the implementation", zap.String("token", tokenString))
			return err
		}
	}

	// Check the optional GetNotBefore field
	startDate, err := token.Claims.GetNotBefore()
	if err == nil {
		if startDate.After(time.Now()) {
			return gojwt.ErrTokenNotValidYet
		}
	}

	// Grab the mandatory expiration time
	expirationDate, err := token.Claims.GetExpirationTime()
	if err != nil {
		return err
	}

	// Check if the token is about to expire
	if expirationDate.Before(time.Now().Add(ExpiryOffset)) {
		return gojwt.ErrTokenExpired
	}

	// Everything is fine!
	return nil
}

func PopulateTokenPairFromBody(tokens *TokenPair, body []byte) {
	if len(body) == 0 {
		return
	}

	var jsonTokens TokenReply
	if err := json.Unmarshal(body, &jsonTokens); err != nil {
		// no error this does not matter
		log.Error("got invalid json reply from server", zap.Error(err))
		return
	}

	// Set the tokens
	tokens.Refresh = jsonTokens.RefreshToken
	tokens.Auth = jsonTokens.AccessToken
}

func PopulateTokenPairFromCookies(tokens *TokenPair, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		switch cookie.Name {
		case RefreshTokenCookieName:
			log.Debug("found refresh token in the response cookies")
			tokens.Refresh = cookie.Value
		case AuthTokenCookieName:
			log.Debug("found auth token in the response cookies")
			tokens.Auth = cookie.Value
		default:
			// skip cookie
			continue
		}

		// Terminate early if we already got auth and refresh token
		if tokens.FullPair() {
			return
		}
	}
}
