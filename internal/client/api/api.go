package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/internal/client/api/jwt"
	"disco.cs.uni-kl.de/apogee/internal/client/config"
	"disco.cs.uni-kl.de/apogee/pkg/log"

	"github.com/imroc/req/v3"
)

type RestAPI struct {
	client *req.Client
	conf   *config.Manager

	// Bearer refresh related
	jwtCtx     *context.Context
	authtoken  atomic.String
	mu         sync.Mutex
	cv         *sync.Cond
	refreshing bool
}

type CTXKey string

const ReqCtxSkipOnBeforeHook = CTXKey("CtxSkipReqPreHook")

var (
	ErrJWTRefreshTokenInvalid = errors.New("the refresh token is invalid")
)

// refreshBearerTokens grabs a new token pair from the server.
// It assumes refresh token rotation, that means a refresh token can only be used once
// and a new refresh token is returned along with the new auth token.
func (r *RestAPI) refreshBearerTokens() (jwt.TokenPair, error) {
	var tokens jwt.TokenPair

	// Verify the refresh token before we send a request
	refreshToken := r.conf.Api().RefreshToken()
	if err := jwt.Validate(refreshToken); err != nil {
		log.Error("refresh token not valid, wont be able to continue", zap.NamedError("reason", err))
		return tokens, ErrJWTRefreshTokenInvalid
	}

	// Create a new context
	jwtCtx, abortRefresh := context.WithCancel(
		// Create a context with magic value so we can skip the on-before-hook
		context.WithValue(context.Background(), ReqCtxSkipOnBeforeHook, true),
	)
	r.jwtCtx = &jwtCtx

	// Reset the context back to nil
	defer func() {
		r.jwtCtx = nil
	}()

	resp, err := r.client.R().
		// Use the refresh token for this request, so we can obtain a new auth and refresh token
		SetBearerAuthToken(refreshToken).
		// Cancel the retries if we got an 401
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil || (resp != nil && resp.StatusCode == http.StatusUnauthorized) {
				// Cancel the context, makes no sense to keep retrying
				abortRefresh()
			}
		}).
		Post(JwtRefreshEndpoint)

	// Return a proper error so we can work with
	if err != nil || resp.IsError() {
		return tokens, ErrorFromResponse(err, resp)
	}

	// If the request has a body, try obtaining the tokens from there first
	jwt.PopulateTokenPairFromBody(&tokens, resp.Bytes())

	// Try cookies as a fallback
	if !tokens.FullPair() {
		jwt.PopulateTokenPairFromCookies(&tokens, resp.Cookies())
	}

	// No full pair? The server might have sent a body that wasnt valid
	if !tokens.FullPair() {
		return tokens, jwt.ErrTokenMissing
	}

	return tokens, nil
}

// Get the current bearer token value
func (r *RestAPI) getBearerTokenValue(req *req.Request) string {
	if req == nil || req.Headers == nil {
		return ""
	}

	_, token, found := strings.Cut(req.Headers.Get("Authorization"), " ")
	if !found {
		return ""
	}

	return token
}

func (r *RestAPI) doBearerRefreshIfNeeded(request *req.Request) error {
	// Skip the request if it contains a custom (overwritten) bearer value
	requestBearer := r.getBearerTokenValue(request)
	if len(requestBearer) > 0 {
		return nil
	}

	// If the auth token we have stored is still valid, continue
	var err error
	if err = jwt.Validate(r.authtoken.Load()); err == nil {
		return nil
	}

	// Else acquire the lock
	r.mu.Lock()

	// Wait for other goroutine to finish refreshing the token, .Wait() locks before returning
	wasRefreshed := false
	for r.refreshing {
		wasRefreshed = true
		r.cv.Wait()
	}

	// If another routine refreshed the tokens, modify this request and return
	if wasRefreshed {
		log.Debug("bearer token updated for pending request")
		// todo: not sure if thats required, but better safe than sorry for now
		request.SetBearerAuthToken(r.authtoken.Load())

		// Unlock early
		r.mu.Unlock()
		return nil
	}

	// Time to refresh, lets register the cleanup logic
	defer func() {
		r.refreshing = false
		r.cv.Broadcast()
		r.mu.Unlock()
	}()

	// fixme what do we do if the token validation returns NotValidYet?
	log.Info("bearer authentication token not valid, refreshing", zap.NamedError("reason", err))
	tokens, err := r.refreshBearerTokens()

	// Check if the refresh attempt failed
	if err != nil {
		log.Error("jwt refresh failed", zap.NamedError("reason", err))
		// Grab the response error if it exists
		if reqErr, ok := err.(*ResponseError); ok {
			// If its forbidden / unauthorized we are done here
			switch reqErr.Code {
			case http.StatusForbidden:
				fallthrough
			case http.StatusUnauthorized:
				return ErrJWTRefreshTokenInvalid
			}
		}

		// We got some other error, cleanup
		return err
	}

	// Modify the current request bcs. the headers were already set
	request.SetBearerAuthToken(tokens.Auth)

	// Modify the common client, so new requests will use the right token
	r.client.SetCommonBearerAuthToken(tokens.Auth)
	r.authtoken.Store(tokens.Auth)

	// Save the new refresh token in the config
	r.conf.Api().SetRefreshToken(tokens.Refresh)
	r.conf.Save()

	log.Info("modified request authorization header and saved new refresh token")

	// cleanup
	return nil
}

func (r *RestAPI) SetUpJWTAuth() {
	// Prepare the required sync condition
	r.cv = sync.NewCond(&r.mu)

	r.client.OnBeforeRequest(func(_ *req.Client, request *req.Request) error {
		// If the request is designed to skip the OnBefore hook, let it through
		// Currently this is the case for the jwt refresh operation
		ctx := request.Context()
		skipHook, ok := ctx.Value(ReqCtxSkipOnBeforeHook).(bool)
		if ok && skipHook {
			return nil
		}

		// If we already know that the token expired before the request, refresh it!
		return r.doBearerRefreshIfNeeded(request)
	})

	// Register a hook for the retry logic
	// It is guaranteed to never get called with a bearer refresh request.
	r.client.SetCommonRetryHook(func(resp *req.Response, err error) {
		// If we encountered an error, thats nothing to process for us
		if err != nil || resp.StatusCode != http.StatusUnauthorized {
			return
		}

		r.doBearerRefreshIfNeeded(resp.Request)
	})
}

func NewRestAPI(conf *config.Manager, debug bool) (*RestAPI, error) {
	a := RestAPI{}
	a.conf = conf

	//set up the connection
	a.client = req.C()

	if debug {
		a.client.EnableDebugLog()
	}

	apiConf := conf.Api()

	// Set up the api base-url
	a.client.SetBaseURL(apiConf.Url())

	// Set up the certificate and authentication
	rootCert := apiConf.RootCertificate()
	if len(rootCert) > 0 {
		a.client.SetRootCertsFromFile(rootCert)
	}

	if len(apiConf.RefreshToken()) > 0 {
		log.Info("using bearer authorization scheme")

		// Set up the retry hook
		a.SetUpJWTAuth()
	} else if apiConf.HasBasicAuth() {
		// Use the sensor name as fallback
		username, password := apiConf.BasicAuth()

		log.Info("using basic auth mechanism", zap.String("username", username))
		a.client.SetCommonBasicAuth(username, password)
	} else {
		log.Warn("no/invalid api authentication scheme specified")
	}

	if apiConf.AllowInsecure() {
		// Skip TLS verification upon request
		a.client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

		log.Warn("!WARNING WARNING WARNING! DISABLED TLS CERTIFICATE VERIFICATION! !WARNING WARNING WARNING!")
	}

	// Some connection configurations
	a.client.SetTimeout(RequestTimeout)
	a.client.SetCommonRetryCount(3)
	a.client.SetCommonRetryBackoffInterval(RequestRetryMinWaitTime, RequestRetryMaxWaitTime)

	// Disable cookies
	a.client.SetCookieJar(nil)

	return &a, nil
}

func (a *RestAPI) GetBaseURL() string {
	if a.client == nil {
		log.Panic("no client, cant get base url")
	}

	return a.client.BaseURL
}

// GetClient Use this for tests to set the transport to mock
func (a *RestAPI) GetClient() *req.Client {
	return a.client
}

type ResponseError struct {
	Status string
	Body   []byte
	Code   int
}

// Error converts the response error to string, but does not print body!
func (e *ResponseError) Error() string {
	return fmt.Sprintf("code: %d status: %s", e.Code, e.Status)
}

// ErrrFromResponse provides properly typed errors for further handling
func ErrorFromResponse(err error, resp *req.Response) error {
	// If an error was encountered, relay it unwrapped
	if err != nil {
		return err
	}

	// everything okay
	if resp.IsSuccess() {
		return nil
	}

	// Check if there was an underlying error
	respErr := resp.Error()
	if respErr != nil {
		return respErr.(error)
	}

	// Default response error
	return &ResponseError{
		Code:   resp.StatusCode,
		Status: resp.Status,
		Body:   resp.Bytes(),
	}
}

func (r *RestAPI) PutSensorUpdate(status SensorStatus) error {
	resp, err := r.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + r.conf.Api().SensorName())

	return ErrorFromResponse(err, resp)
}

func (r *RestAPI) GetJobs() ([]FixedJob, error) {
	respCont := FixedJobResponse{}
	resp, err := r.client.R().
		SetHeader("Accept", "application/json").
		SetResult(&respCont).
		Get("fixedjobs/" + r.conf.Api().SensorName())

	if err != nil {
		return []FixedJob{}, err
	}

	return respCont.Data, ErrorFromResponse(nil, resp)
}

// fixme why is this using the job name instead of the ID?
func (r *RestAPI) PutJobUpdate(jobName string, status string) error {
	if status != "running" && status != "finished" && status != "failed" {
		return errors.New("only status 'running', 'finished' or 'failed' allowed")
	}
	resp, err := r.client.R().
		Put("fixedjobs/" + r.conf.Api().SensorName() + "?job_name=" + jobName + "&status=" + status)

	return ErrorFromResponse(err, resp)
}

func (r *RestAPI) PostSensorData(ctx context.Context, jobName string, fileName string, filePath string) error {
	// Upload the file
	resp, err := r.client.R().
		// Set the context so we can abort
		SetContext(ctx).
		SetFile("in_file", filePath).
		SetUploadCallbackWithInterval(func(info req.UploadInfo) {
			log.Info("intermediate upload progress", zap.String("file", info.FileName), zap.Float64("pct", float64(info.UploadedSize)/float64(info.FileSize)*100.0))
		}, 100*time.Millisecond).
		Post("data/" + r.conf.Api().SensorName() + "/" + jobName)

	// gather information for possible upload-timeout errors
	log.Debug("end uploading the job-zip-file", zap.String("fileName", fileName))
	return ErrorFromResponse(err, resp)
}
