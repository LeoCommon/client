package jwt

import (
	"encoding/json"
	"net/http"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client/api/jwt/misc"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	gojwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

func Validate(tokenString string) error {
	// token is empty, this happens on restarts when only the refresh token is available
	if len(tokenString) == 0 {
		return misc.ErrTokenMissing
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
	if expirationDate.Before(time.Now().Add(misc.ExpiryOffset)) {
		return gojwt.ErrTokenExpired
	}

	// Everything is fine!
	return nil
}

func PopulateTokenPairFromBody(tokens *misc.TokenPair, body []byte) {
	if len(body) == 0 {
		return
	}

	// Set the tokens
	if err := json.Unmarshal(body, &tokens); err != nil {
		// no error this does not matter
		log.Error("got invalid json reply from server", zap.Error(err))
		return
	}
}

type CookieNameFunc func() (access string, refresh string)

func PopulateTokenPairFromCookies(tokens *misc.TokenPair, names CookieNameFunc, cookies []*http.Cookie) {
	accessName, refreshName := names()

	for _, cookie := range cookies {
		switch cookie.Name {
		case refreshName:
			log.Debug("found refresh token in the response cookies")
			tokens.Refresh = cookie.Value
		case accessName:
			log.Debug("found auth token in the response cookies")
			tokens.Access = cookie.Value
		default:
			// skip cookie
			continue
		}

		// Terminate early if we already got auth and refresh token
		if tokens.FullTokenPair() {
			return
		}
	}
}
