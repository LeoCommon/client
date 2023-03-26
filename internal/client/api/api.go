package api

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/internal/client/api/jwt"
	"disco.cs.uni-kl.de/apogee/internal/client/config"
	"disco.cs.uni-kl.de/apogee/pkg/log"

	"github.com/go-resty/resty/v2"
)

type RestAPI struct {
	wg                   *sync.WaitGroup
	jwtRefreshInProgress atomic.Bool
	client               *resty.Client
	conf                 *config.Manager
}

var (
	ErrJWTRefreshTokenInvalid = errors.New("the refresh token is invalid")
	ErrJWTRefreshInProgress   = errors.New("a refresh opeation is already in progress")
)

// refreshJWT grabs a new token pair from the server.
// It assumes refresh token rotation, that means a refresh token can only be used once
// and a new refresh token is returned along with the new auth token.
func (r *RestAPI) refreshJWT() error {
	if r.jwtRefreshInProgress.Load() {
		return ErrJWTRefreshInProgress
	}

	// Guard the refresh operation
	r.jwtRefreshInProgress.Store(true)
	defer r.jwtRefreshInProgress.Store(false)

	// a token pair, might consist of refresh and/or auth token
	tokens := jwt.TokenPair{}

	// Verify the refresh token before we send a request
	refreshToken := r.conf.Api().RefreshToken()
	if err := jwt.Validate(refreshToken); err != nil {
		log.Error("refresh token not valid, wont be able to continue", zap.NamedError("reason", err))
		return ErrJWTRefreshTokenInvalid
	}

	// If the refresh completed, make all other api requests resume
	defer r.wg.Done()

	resp, err := r.client.R().
		// Use the refresh token for this request, so we can obtain a new auth and refresh token
		SetAuthToken(refreshToken).
		Post(JwtRefreshEndpoint)

	// Return a proper error we can work with
	if err != nil {
		return ErrorFromResponse(resp)
	}

	// If the request has a body, try it first
	jwt.PopulateTokenPairFromBody(&tokens, resp.Body())

	// Try cookies as a fallback
	if !tokens.FullPair() {
		jwt.PopulateTokenPairFromCookies(&tokens, resp.Cookies())
	}

	// No full pair? The server might have sent a body that wasnt valid
	if !tokens.FullPair() {
		return jwt.ErrTokenMissing
	}

	log.Info("saving new refresh token")
	r.client.SetAuthToken(tokens.Auth)
	r.conf.Api().SetRefreshToken(tokens.Refresh)
	r.conf.Save()
	return nil
}

func (r *RestAPI) SetUpJWTAuth() {
	// If we already know that the token expired before the request, refresh it!
	r.client.OnBeforeRequest(func(client *resty.Client, request *resty.Request) error {
		// If this is a request to the jwt endpoint
		// 1) make all other requests wait
		// 2) return early to avoid endlessly looping refreshes
		var err error
		if strings.Contains(request.URL, JwtRefreshEndpoint) {
			r.wg.Add(1)
			return nil
		}

		// If there is a pending operation in progress, wait for it to complete
		r.wg.Wait()

		// If the access token is valid do nothing
		if err = jwt.Validate(client.Token); err == nil {
			return nil
		}

		// If there is a refresh operation pending, wait for it to complete
		log.Debug("jwt is not valid", zap.NamedError("reason", err))

		// (Try) to grab a new token
		// fixme what do we do if the token validation returns NotValidYet?
		refreshError := r.refreshJWT()
		if refreshError == nil {
			// Modify the request token here as its already in-preparation
			request.Token = client.Token
			log.Info("refreshed jwt tokens before executing an api request")
			return nil
		}

		// Prevent double refresh
		if refreshError == ErrJWTRefreshInProgress {
			log.Warn("prevented refresh of jwt, was already in progress")
			return refreshError
		}

		log.Error("ahead of time jwt refresh failed", zap.NamedError("reason", err))

		// Unrecoverable error
		if refreshError == ErrJWTRefreshTokenInvalid {
			return refreshError
		}

		// Grab the request error if it exists
		if reqErr, ok := refreshError.(*ResponseError); ok {
			// If its forbidden / unauthorized we are done here
			switch reqErr.Code {
			case http.StatusForbidden:
				fallthrough
			case http.StatusUnauthorized:
				return ErrJWTRefreshTokenInvalid
			}
		}

		// Dont abort the request if the jwt refresh fails due to other reasons
		return nil
	})

	// If we get StatusUnauthorized from the server we might have an outdated token
	r.client.OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
		// fixme: clarify that if the response is nil, the request was cancelled?
		if resp == nil {
			return ErrJWTRefreshTokenInvalid
		}

		// If we get an unauthorized, try to refresh the token, if its not the jwt endpoint
		isJWTRefreshEndoint := strings.Contains(resp.Request.URL, JwtRefreshEndpoint)
		unauthorized := resp.StatusCode() == http.StatusUnauthorized
		if isJWTRefreshEndoint && unauthorized {
			return ErrJWTRefreshTokenInvalid
		}

		// If its unauthorized, but not the JWT refresh endpoint, try to refresh
		if unauthorized {
			err := r.refreshJWT()
			if err != nil {
				log.Error("token refresh on retry failed", zap.NamedError("reason", err))
			} else {
				log.Info("refreshed jwt tokens after receiving 401 from the api")
			}
		}

		return nil // if its success otherwise return error
	})
}

func NewRestAPI(conf *config.Manager) (*RestAPI, error) {
	a := RestAPI{}
	a.conf = conf

	//set up the connection
	a.client = resty.New()
	a.wg = &sync.WaitGroup{}

	apiConf := conf.Api()

	// Set up the api base-url
	a.client.SetBaseURL(apiConf.Url())

	// Set up the certificate and authentication
	rootCert := apiConf.RootCertificate()
	if len(rootCert) > 0 {
		a.client.SetRootCertificate(rootCert)
	}

	if len(apiConf.RefreshToken()) > 0 {
		log.Info("using bearer authorization scheme")

		// Set up the retry hook
		a.SetUpJWTAuth()
	} else if apiConf.HasBasicAuth() {
		// Use the sensor name as fallback
		username, password := apiConf.BasicAuth()

		log.Info("using basic auth mechanism", zap.String("username", username))
		a.client.SetBasicAuth(username, password)
	} else {
		log.Warn("no/invalid api authentication scheme specified")
	}

	if apiConf.AllowInsecure() {
		// Skip TLS verification upon request
		a.client.SetTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		})

		log.Warn("!WARNING WARNING WARNING! DISABLED TLS CERTIFICATE VERIFICATION! !WARNING WARNING WARNING!")
	}

	// Some connection configurations
	a.client.SetTimeout(RequestTimeout)
	a.client.SetRetryCount(3)
	a.client.SetRetryWaitTime(RequestRetryWaitTime)
	a.client.SetRetryMaxWaitTime(RequestRetryMaxWaitTime)

	return &a, nil
}

func (a *RestAPI) GetBaseURL() string {
	if a.client == nil {
		log.Panic("no client, cant get base url")
	}

	return a.client.BaseURL
}

// SetTransport Use this for tests to set the transport to mock
func (a *RestAPI) SetTransport(transport http.RoundTripper) {
	if a.client == nil {
		log.Panic("no client, cant set transport")
	}

	a.client.SetTransport(transport)
}

// GetClient Use this for tests to set the transport to mock
func (a *RestAPI) GetClient() *resty.Client {
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
func ErrorFromResponse(resp *resty.Response) error {
	// everything okay
	if resp.IsSuccess() {
		return nil
	}

	// Check if there is a underlying error
	respErr := resp.Error()
	if respErr != nil {
		return respErr.(error)
	}

	// Default response error
	return &ResponseError{
		Code:   resp.StatusCode(),
		Status: resp.Status(),
		Body:   resp.Body(),
	}
}

func (r *RestAPI) PutSensorUpdate(status SensorStatus) error {
	resp, err := r.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + r.conf.Api().SensorName())
	if err != nil {
		return err
	}

	return ErrorFromResponse(resp)
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

	return respCont.Data, ErrorFromResponse(resp)
}

// fixme why is this using the job name instead of the ID?
func (r *RestAPI) PutJobUpdate(jobName string, status string) error {
	if status != "running" && status != "finished" && status != "failed" {
		return errors.New("only status 'running', 'finished' or 'failed' allowed")
	}
	resp, err := r.client.R().
		Put("fixedjobs/" + r.conf.Api().SensorName() + "?job_name=" + jobName + "&status=" + status)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	return ErrorFromResponse(resp)
}

func (r *RestAPI) PostSensorData(jobName string, fileName string, filePath string) error {
	// fixme: Disable the timeout for this request (really bad)
	r.client.SetTimeout(0)
	defer func() {
		// Restore the timeout
		r.client.SetTimeout(RequestTimeout)
	}()

	// Upload the file
	resp, err := r.client.R().
		SetFile("in_file", filePath).
		Post("data/" + r.conf.Api().SensorName() + "/" + jobName)

	if err != nil {
		log.Error(err.Error())
		return err
	}

	// gather information for possible upload-timeout errors
	log.Debug("end uploading the job-zip-file", zap.String("fileName", fileName))
	return ErrorFromResponse(resp)
}
