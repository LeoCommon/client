package api

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/pkg/log"

	"github.com/go-resty/resty/v2"
)

// Config contains the api configuration options
type Config struct {
	SensorName string
	Url        string

	Security struct {
		Basic *struct {
			Username string
			Password string
		} `toml:"basic,omitempty"`
		Bearer *struct {
			Scheme string
			Token  string
		} `toml:"bearer,omitempty"`

		RootCertificate string `toml:"root_certificate"`
		AllowInsecure   bool   `toml:"allow_insecure"`
	}
}

type RestAPI struct {
	resty *resty.Client
	conf  Config
}

func NewRestAPI(conf Config) (*RestAPI, error) {
	a := RestAPI{}
	a.conf = conf

	//set up the connection
	a.resty = resty.New()
	// Set up the api base-url
	a.resty.SetBaseURL(conf.Url)
	// Set up the certificate and authentication

	if conf.Security.RootCertificate != "" {
		a.resty.SetRootCertificate(conf.Security.RootCertificate)
	}

	if conf.Security.Bearer != nil {
		token := conf.Security.Bearer.Token
		if scheme := conf.Security.Bearer.Scheme; scheme != "" {
			log.Info("using custom authorization scheme", zap.String("scheme", scheme))
			a.resty.SetAuthScheme(fmt.Sprintf("Authorization: %s %s", scheme, token))
		} else {
			log.Info("using bearer authorization scheme")
			a.resty.SetAuthToken(token)
		}
	} else if conf.Security.Basic != nil {
		// Use the sensor name as fallback
		username := conf.Security.Basic.Username
		if username == "" {
			username = conf.SensorName
		}

		log.Info("using basic auth mechanism", zap.String("username", username))
		a.resty.SetBasicAuth(username, conf.Security.Basic.Password)
	} else {
		log.Info("using no authentication for the API!")
	}

	if conf.Security.AllowInsecure {
		// Skip TLS verification upon request
		a.resty.SetTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		})

		log.Warn("!WARNING WARNING WARNING! DISABLED TLS CERTIFICATE VERIFICATION! !WARNING WARNING WARNING!")
	}

	// Some connection configurations
	a.resty.SetTimeout(RequestTimeout)
	a.resty.SetRetryCount(3)
	a.resty.SetRetryWaitTime(RequestRetryWaitTime)
	a.resty.SetRetryMaxWaitTime(RequestRetryMaxWaitTime)

	return &a, nil
}

func (a *RestAPI) GetBaseURL() string {
	if a.resty == nil {
		log.Panic("no client, cant get base url")
	}

	return a.resty.BaseURL
}

// SetTransport Use this for tests to set the transport to mock
func (a *RestAPI) SetTransport(transport http.RoundTripper) {
	if a.resty == nil {
		log.Panic("no client, cant set transport")
	}

	a.resty.SetTransport(transport)
}

// GetClient Use this for tests to set the transport to mock
func (a *RestAPI) GetClient() *resty.Client {
	return a.resty
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

func (a *RestAPI) PutSensorUpdate(status SensorStatus) error {
	resp, err := a.resty.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + a.conf.SensorName)
	if err != nil {
		return err
	}

	return ErrorFromResponse(resp)
}

func (a *RestAPI) GetJobs() ([]FixedJob, error) {
	respCont := FixedJobResponse{}
	resp, err := a.resty.R().
		SetHeader("Accept", "application/json").
		SetResult(&respCont).
		Get("fixedjobs/" + a.conf.SensorName)

	if err != nil {
		return []FixedJob{}, err
	}

	return respCont.Data, ErrorFromResponse(resp)
}

func (a *RestAPI) PutJobUpdate(jobName string, status string) error {
	if status != "running" && status != "finished" && status != "failed" {
		return errors.New("only status 'running', 'finished' or 'failed' allowed")
	}
	resp, err := a.resty.R().
		Put("fixedjobs/" + a.conf.SensorName + "?job_name=" + jobName + "&status=" + status)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	return ErrorFromResponse(resp)
}

func (a *RestAPI) PostSensorData(jobName string, fileName string, filePath string) error {
	// fixme: Disable the timeout for this request (really bad)
	a.resty.SetTimeout(0)
	defer func() {
		// Restore the timeout
		a.resty.SetTimeout(RequestTimeout)
	}()

	// Upload the file
	resp, err := a.resty.R().
		SetFile("in_file", filePath).
		Post("data/" + a.conf.SensorName + "/" + jobName)

	if err != nil {
		log.Error(err.Error())
		return err
	}

	// gather information for possible upload-timeout errors
	log.Debug("end uploading the job-zip-file", zap.String("fileName", fileName))
	return ErrorFromResponse(resp)
}
