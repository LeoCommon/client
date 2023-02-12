package api

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"

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
const ClientTimeout = 10 * time.Second
const ClientRetryWaitTime = 10 * time.Second

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
	client.SetTimeout(ClientTimeout)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(ClientRetryWaitTime)
	client.SetRetryMaxWaitTime(ClientRetryWaitTime)

}

func PutSensorUpdate(status SensorStatus) error {
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + sensorName)
	if err != nil {
		apglog.Error(err.Error())
		return err
	}
	if resp.StatusCode() != 200 {
		apglog.Info("PutSensorUpdate.ResponseStatus = " + resp.Status() + resp.String())
		return errors.New("PutSensorUpdate.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		apglog.Info("PutSensorUpdate.Response-internal error: " + resp.String())
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
		apglog.Error(err.Error())
		return []FixedJob{}, err
	} else if resp.StatusCode() != 200 {
		apglog.Info("GetJobs.ResponseStatus = " + resp.Status())
		return respCont.Data, errors.New("GetJobs.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		apglog.Info("GetJobs.Response-internal error: " + resp.String())
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
		apglog.Error(err.Error())
		return err
	} else if resp.StatusCode() != 200 {
		apglog.Info("PutJobUpdate.ResponseStatus = " + resp.Status())
		return errors.New("PutJobUpdate.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		apglog.Info("PutJobUpdate.Response-internal error: " + resp.String())
		return errors.New("PutJobUpdate.Response-internal error: " + resp.String())
	}
	return nil
}

func PostSensorData(jobName string, fileName string, filePath string) error {
	// gather information for possible upload-timeout errors
	fi, err := os.Stat(filePath)
	if err != nil {
		apglog.Error("Could not get size of job-zip-file: " + err.Error())
	} else {
		apglog.Debug("start uploading the job-zip-file", zap.String("fileName", fileName), zap.Int64("fileSize[Byte]", fi.Size()))
	}
	//load the file that should be sent
	fileData, err := os.Open(filePath)
	if err != nil {
		apglog.Error("Error finding job-file: " + err.Error())
	}
	defer fileData.Close()
	rBody := &bytes.Buffer{}
	bWriter := multipart.NewWriter(rBody)
	part, err := bWriter.CreateFormFile("in_file", fileName)
	if err != nil {
		apglog.Fatal("Error loading job-file: " + err.Error())
	}
	defer fileData.Close()
	io.Copy(part, fileData)
	bWriter.Close()

	// use this as temporary solution to avoid timeouts during uploads
	// TODO: find a proper solution
	client.SetTimeout(10 * ClientTimeout)

	resp, err := client.R().
		SetHeader("Content-Type", bWriter.FormDataContentType()).
		SetBody(rBody).
		Post("data/" + sensorName + "/" + jobName)
	client.SetTimeout(ClientTimeout) // revert the change
	if err != nil {
		apglog.Error(err.Error())
		return err
	} else if resp.StatusCode() != 200 {
		apglog.Info("PutSensorData.ResponseStatus = " + resp.Status())
		return errors.New("PutSensorData.ResponseStatus = " + resp.Status())
	}
	if strings.Contains(resp.String(), "error") {
		apglog.Info("PutSensorData.Response-internal error: " + resp.String())
		return errors.New("PutSensorData.Response-internal error: " + resp.String())
	}
	// gather information for possible upload-timeout errors
	apglog.Debug("end uploading the job-zip-file", zap.String("fileName", fileName))

	return nil
}
