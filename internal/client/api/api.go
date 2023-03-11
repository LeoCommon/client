package api

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

func (a *RestAPI) PutSensorUpdate(status SensorStatus) error {
	resp, err := a.resty.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + a.conf.SensorName)
	if err != nil {
		log.Error(err.Error())
		return err
	}
	if resp.StatusCode() != 200 {
		log.Info("PutSensorUpdate.ResponseStatus = " + resp.Status() + resp.String())
		return errors.New("PutSensorUpdate.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		log.Info("PutSensorUpdate.Response-internal error: " + resp.String())
		return errors.New("PutSensorUpdate.Response-internal error: " + resp.String())
	}
	return nil
}

func (a *RestAPI) GetJobs() ([]FixedJob, error) {
	respCont := FixedJobResponse{}
	resp, err := a.resty.R().
		SetHeader("Accept", "application/json").
		SetResult(&respCont).
		Get("fixedjobs/" + a.conf.SensorName)
	if err != nil {
		log.Error(err.Error())
		return []FixedJob{}, err
	} else if resp.StatusCode() != 200 {
		log.Info("GetJobs.ResponseStatus = " + resp.Status())
		return respCont.Data, errors.New("GetJobs.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		log.Info("GetJobs.Response-internal error: " + resp.String())
		return respCont.Data, errors.New("GetJobs.Response-internal error: " + resp.String())
	}
	return respCont.Data, nil
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
	} else if resp.StatusCode() != 200 {
		log.Info("PutJobUpdate.ResponseStatus = " + resp.Status())
		return errors.New("PutJobUpdate.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		log.Info("PutJobUpdate.Response-internal error: " + resp.String())
		return errors.New("PutJobUpdate.Response-internal error: " + resp.String())
	}
	return nil
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
	} else if resp.StatusCode() != 200 {
		log.Info("PutSensorData.ResponseStatus = " + resp.Status())
		return errors.New("PutSensorData.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		log.Info("PutSensorData.Response-internal error: " + resp.String())
		return errors.New("PutSensorData.Response-internal error: " + resp.String())
	}

	// gather information for possible upload-timeout errors
	log.Debug("end uploading the job-zip-file", zap.String("fileName", fileName))

	return nil
}
