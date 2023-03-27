package jwt

import (
	"context"
	"net/http"
	"sync"

	"disco.cs.uni-kl.de/apogee/internal/client/api/helpers"
	"disco.cs.uni-kl.de/apogee/internal/client/api/jwt/misc"
	"disco.cs.uni-kl.de/apogee/internal/client/config"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"github.com/imroc/req/v3"
	"go.uber.org/zap"
)

type JwtHandler struct {
	mu    sync.Mutex
	cv    *sync.Cond
	c     *req.Client
	apiCM *config.ApiConfigManager

	// This is a copy of the bearer settings
	conf       config.AuthBearerSettings
	refreshing bool
}

func NewJWTHandler(cm *config.ApiConfigManager, c *req.Client) (*JwtHandler, error) {
	// Get the initial data
	conf := cm.C()
	bearerSettings := conf.Auth.Bearer
	if bearerSettings == nil {
		return nil, misc.ErrInvalidJWTSettings
	}

	j := JwtHandler{
		c:     c,
		apiCM: cm,

		// Copy by value
		conf: *bearerSettings,
	}

	// Fallback to the default refresh endpoint if nothing was specified
	if bearerSettings.RefreshEndpoint == "" {
		j.conf.RefreshEndpoint = misc.DefaultRefreshEndpoint
	}

	// If no source is specified use only body
	if bearerSettings.Sources == nil {
		j.conf.Sources = &[]config.BearerTokenSource{
			config.BearerSourceBody,
		}
	}

	// If no bearer scheme is specified use the default one
	if bearerSettings.HeaderSettings.Scheme == "" {
		j.conf.HeaderSettings.Scheme = misc.DefaultSchemeName
	}

	j.cv = sync.NewCond(&j.mu)

	// Initialize with the modified settings
	j.init()

	return &j, nil
}

func (j *JwtHandler) init() {
	// Create the hooks
	j.c.OnBeforeRequest(func(_ *req.Client, request *req.Request) error {
		// If the request is designed to skip the OnBefore hook, let it through
		// Currently this is the case for the jwt refresh operation
		ctx := request.Context()
		skipHook, ok := ctx.Value(helpers.ReqCtxSkipOnBeforeHook).(bool)
		if ok && skipHook {
			return nil
		}

		// If we already know that the token expired before the request, refresh it!
		return j.DoBearerRefreshIfNeeded(request)
	})

	// Register a hook for the retry logic
	// It is guaranteed to never get called with a bearer refresh request.
	j.c.SetCommonRetryHook(func(resp *req.Response, err error) {
		// If we encountered an error, thats nothing to process for us
		if err != nil || resp.StatusCode != http.StatusUnauthorized {
			return
		}

		j.DoBearerRefreshIfNeeded(resp.Request)
	})

	// Disable cookies entirely if the cookie source is disabled
	if !j.conf.CookieSourceEnabled() {
		log.Info("disabling cookie support")
		j.c.SetCookieJar(nil)
	}

	// Prepare the initial header
	j.c.SetCommonHeader(j.getCustomBearerHeader(j.conf.Access))
}

// getCustomBearerHeader allows modification of the Scheme
func (j *JwtHandler) getCustomBearerHeader(token string) (string, string) {
	return "Authorization", j.conf.HeaderSettings.Scheme + " " + token
}

// refreshBearerTokens grabs a new token pair from the server.
// It assumes refresh token rotation, that means a refresh token can only be used once
// and a new refresh token is returned along with the new auth token.
func (j *JwtHandler) RefreshBearerTokens() (misc.TokenPair, error) {
	var tokens misc.TokenPair

	// Verify the refresh token before we send a request
	if err := Validate(j.conf.Refresh); err != nil {
		log.Error("refresh token not valid, wont be able to continue", zap.NamedError("reason", err))
		return tokens, misc.ErrRefreshTokenInvalid
	}

	// Create a new context
	ctx, cancelRefreshRequest := context.WithCancel(
		// Create a context with magic value so we can skip the on-before-hook
		context.WithValue(context.Background(), helpers.ReqCtxSkipOnBeforeHook, true),
	)
	defer cancelRefreshRequest()

	resp, err := j.c.R().
		// Pass the context to the request
		SetContext(ctx).
		// If we are not using cookies, use the refresh token, so we can obtain a new auth and refresh token
		SetHeader(j.getCustomBearerHeader(j.conf.Refresh)).
		// Override the retry hook, so we can stop retrying if we got an 401
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil || (resp != nil && resp.StatusCode == http.StatusUnauthorized) {
				// Cancel the context, makes no sense to keep retrying
				cancelRefreshRequest()
			}
		}).
		Post(j.conf.RefreshEndpoint)

	// Check for errors
	if err != nil || resp.IsErrorState() {
		return tokens, helpers.ErrorFromResponse(err, resp)
	}

	// If the body source is enabled, use it first
	if j.conf.BodySourceEnabled() {
		PopulateTokenPairFromBody(&tokens, resp.Bytes())
	}

	// Try cookies as a fallback if activated
	if !tokens.FullTokenPair() && j.conf.CookieSourceEnabled() {
		// Fill in the cookie names
		PopulateTokenPairFromCookies(&tokens, func() (access string, refresh string) {
			access = j.conf.CookieSettings.AccessTokenName
			if access == "" {
				access = misc.DefaultAuthTokenCookieName
			}

			refresh = j.conf.CookieSettings.RefreshTokenName
			if refresh == "" {
				refresh = misc.DefaultRefreshTokenCookieName
			}

			return
		}, resp.Cookies())
	}

	// No full pair? The server sent something invalid
	if !tokens.FullTokenPair() {
		return tokens, misc.ErrTokenMissing
	}

	return tokens, nil
}

func (j *JwtHandler) DoBearerRefreshIfNeeded(request *req.Request) error {
	// If the auth token we have stored is still valid, continue
	var err error
	if err = Validate(j.conf.Access); err == nil {
		return nil
	}

	// Else acquire the lock
	j.mu.Lock()

	// Wait for other goroutine to finish refreshing the token, .Wait() locks before returning
	wasRefreshed := false
	for j.refreshing {
		wasRefreshed = true
		j.cv.Wait()
	}

	// If another routine refreshed the tokens, modify this request and return
	if wasRefreshed {
		log.Debug("bearer token updated for pending request")
		// todo: not sure if thats required, but better safe than sorry for now
		request.SetBearerAuthToken(j.conf.Access)

		// Unlock early
		j.mu.Unlock()
		return nil
	}

	// Time to refresh, lets register the cleanup logic
	defer func() {
		j.refreshing = false
		j.cv.Broadcast()
		j.mu.Unlock()
	}()

	j.refreshing = true
	log.Info("bearer authentication token not valid, refreshing", zap.NamedError("reason", err))
	tokens, err := j.RefreshBearerTokens()
	// fixme what do we do if the token validation returns NotValidYet?

	// Check if the refresh attempt failed
	if err != nil {
		log.Error("jwt refresh failed", zap.NamedError("reason", err))
		// Grab the response error if it exists
		if reqErr, ok := err.(*helpers.ResponseError); ok {
			// If its forbidden / unauthorized we are done here
			switch reqErr.Code {
			case http.StatusForbidden:
				fallthrough
			case http.StatusUnauthorized:
				return misc.ErrRefreshTokenInvalid
			}
		}

		// We got some other error, cleanup
		return err
	}

	// Update our internal configuration
	j.conf.Access = tokens.Access
	j.conf.Refresh = tokens.Refresh

	// Modify the current request bcs. the headers were most likely already set
	header, value := j.getCustomBearerHeader(tokens.Access)
	request.SetHeader(header, value)
	j.c.SetCommonHeader(header, value)

	// Modify the api client, so new requests will use the right token
	log.Info("modified run-time bearer tokens")

	// Set the new tokens in the global api config, so we can save them
	j.apiCM.Set(func(config *config.ApiConfig) {
		config.Auth.Bearer.Refresh = tokens.Refresh
		config.Auth.Bearer.Access = tokens.Access
	})

	// Save the config
	j.apiCM.Save()

	// cleanup
	return nil
}
