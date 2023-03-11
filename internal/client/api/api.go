package api

import (
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/pkg/log"

	"github.com/go-resty/resty/v2"
)

// Some structs to handle the Json, coming from the server

type FixedJob struct {
	Id        string            `json:"id"`
	Name      string            `json:"name"`
	StartTime int64             `json:"start_time"`
	EndTime   int64             `json:"end_time"`
	Command   string            `json:"command"`
	Arguments map[string]string `json:"arguments"`
	Sensors   []string          `json:"sensors"`
	Status    string            `json:"status"`
	States    map[string]string `json:"states"`
}

type FixedJobResponse struct {
	Data    []FixedJob
	Code    int
	Message string
}

type SensorStatus struct {
	StatusTime         int64   `json:"status_time"`
	LocationLat        float64 `json:"location_lat"`
	LocationLon        float64 `json:"location_lon"`
	OsVersion          string  `json:"os_version"`
	TemperatureCelsius float64 `json:"temperature_celsius"`
	LTE                string  `json:"LTE"`
	WiFi               string  `json:"WiFi"`
	Ethernet           string  `json:"Ethernet"`
}

// Local variables to handle the connection
var sensorName string
var sensorPw string
var client *resty.Client

// Constants of the rest-client

func GetBaseURL() string {
	if client == nil {
		log.Panic("no client, cant get base url")
	}

	return client.BaseURL
}

// Use this for tests to set the transport to mock
func SetTransport(transport http.RoundTripper) {
	if client == nil {
		log.Panic("no client, cant set transport")
	}

	client.SetTransport(transport)
}

// Use this for tests to set the transport to mock
func GetClient() *resty.Client {
	return client
}

func SetupAPI(baseURL string, serverCertFile *string, loginName string, loginPassword string) {
	//get the login credentials from a file
	sensorName = loginName
	sensorPw = loginPassword

	//set up the connection
	client = resty.New()
	// Set up the api base-url
	client.SetBaseURL(baseURL)
	// Set up the certificate and authentication
	if serverCertFile != nil {
		client.SetRootCertificate(*serverCertFile)
	}

	client.SetBasicAuth(sensorName, sensorPw)
	// Some connection configurations
	client.SetTimeout(RequestTimeout)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(RequestRetryWaitTime)
	client.SetRetryMaxWaitTime(RequestRetryMaxWaitTime)
}

func PutSensorUpdate(status SensorStatus) error {
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + sensorName)
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

func GetJobs() ([]FixedJob, error) {
	respCont := FixedJobResponse{}
	resp, err := client.R().
		SetHeader("Accept", "application/json").
		SetResult(&respCont).
		Get("fixedjobs/" + sensorName)
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

func PutJobUpdate(jobName string, status string) error {
	if status != "running" && status != "finished" && status != "failed" {
		return errors.New("only status 'running', 'finished' or 'failed' allowed")
	}
	resp, err := client.R().
		Put("fixedjobs/" + sensorName + "?job_name=" + jobName + "&status=" + status)
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

func PostSensorData(jobName string, fileName string, filePath string) error {
    // fixme: Disable the timeout for this request (really bad)
    client.SetTimeout(0)
    defer func() {
        // Restore the timeout
        client.SetTimeout(RequestTimeout)
    }()

	// Upload the file
	resp, err := client.R().
		SetFile("in_file", filePath).
		Post("data/" + sensorName + "/" + jobName)

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
