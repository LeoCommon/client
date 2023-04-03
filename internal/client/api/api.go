package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	h "disco.cs.uni-kl.de/apogee/internal/client/api/helpers"
	"disco.cs.uni-kl.de/apogee/internal/client/api/jwt"
	"disco.cs.uni-kl.de/apogee/internal/client/config"
	"disco.cs.uni-kl.de/apogee/pkg/log"

	"github.com/imroc/req/v3"
)

type RestAPI struct {
	client *req.Client

	jwt *jwt.JwtHandler

	// Store these for later usage
	conf     *config.Manager
	cm       *config.ApiConfigManager
	clientCM *config.ClientConfigManager
}

func NewRestAPI(conf *config.Manager, debug bool) (*RestAPI, error) {
	a := RestAPI{}
	a.conf = conf

	a.cm = conf.Api()
	a.clientCM = conf.Client()

	//set up the connection
	a.client = req.C()

	if debug {
		a.client.EnableDebugLog()
	}

	// Get a copy of the api config
	apiConf := a.cm.C()

	// Set up the api base-url
	a.client.SetBaseURL(apiConf.Url)

	// Set up the certificate and authentication
	rootCert := apiConf.RootCertificate
	if len(rootCert) > 0 {
		a.client.SetRootCertsFromFile(rootCert)
	}

	if apiConf.Auth.Bearer != nil {
		// Verify that the refresh token is valid
		if err := jwt.Validate(apiConf.Auth.Bearer.Refresh); err != nil {
			log.Error("refresh token validation failed", zap.NamedError("reason", err))
			return nil, fmt.Errorf("trying to use bearer authentication with invalid refresh token")
		}

		log.Info("using bearer authorization")

		// Set up the handler and its hooks
		var err error
		a.jwt, err = jwt.NewJWTHandler(a.cm, a.client)
		if err != nil {
			return nil, err
		}
	} else if apiConf.Auth.Basic != nil {
		username, password := apiConf.Auth.Basic.Credentials()
		log.Info("using basic auth mechanism", zap.String("username", username))
		a.client.SetCommonBasicAuth(username, password)
	} else {
		log.Warn("no/invalid api authentication scheme specified")
	}

	if apiConf.AllowInsecure {
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

func (r *RestAPI) PutSensorUpdate(status SensorStatus) error {
	resp, err := r.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + r.clientCM.C().SensorName)

	return h.ErrorFromResponse(err, resp)
}

func (r *RestAPI) GetJobs() ([]FixedJob, error) {
	respCont := FixedJobResponse{}
	resp, err := r.client.R().
		SetHeader("Accept", "application/json").
		SetSuccessResult(&respCont).
		Get("fixedjobs/" + r.clientCM.C().SensorName)

	if err != nil {
		return []FixedJob{}, err
	}

	return respCont.Data, h.ErrorFromResponse(nil, resp)
}

// fixme why is this using the job name instead of the ID?
func (r *RestAPI) PutJobUpdate(jobName string, status string) error {
	if status != "running" && status != "finished" && status != "failed" {
		return errors.New("only status 'running', 'finished' or 'failed' allowed")
	}
	resp, err := r.client.R().
		Put("fixedjobs/" + r.clientCM.C().SensorName + "?job_name=" + jobName + "&status=" + status)

	return h.ErrorFromResponse(err, resp)
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
		Post("data/" + r.clientCM.C().SensorName + "/" + jobName)

	// gather information for possible upload-timeout errors
	log.Debug("end uploading the job-zip-file", zap.String("fileName", fileName))
	return h.ErrorFromResponse(err, resp)
}
