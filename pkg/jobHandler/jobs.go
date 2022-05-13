package jobHandler

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
)

func writeInFile(fileName string, text string) (os.File, error) {
	f, err := os.Create(fileName)
	if err != nil {
		return os.File{}, err
	}
	w := bufio.NewWriter(f)
	_, err = w.WriteString(text)
	if err != nil {
		return os.File{}, err
	}
	err = w.Flush()
	if err != nil {
		return os.File{}, err
	}
	return *f, nil
}

func GetDefaultSensorStatus() api.SensorStatus {
	status := api.SensorStatus{StatusTime: time.Now().Unix()}
	status.LocationLat = "123.456"
	status.LocationLon = "123.456"
	status.OsVersion = "0.1a"
	status.TemperatureCelsius = 999.999
	status.LTE = "testing"
	status.WiFi = "testing"
	status.Ethernet = "testing"
	return status
}

func PushStatus() error {
	newStatus := GetDefaultSensorStatus()
	err := api.PutSensorUpdate(newStatus)
	return err
}

func ReportFullStatus(jobName string) error {
	newStatus := GetDefaultSensorStatus()
	statusString, err := json.Marshal(newStatus)
	if err != nil {
		apglog.Info("Error encoding the status: " + err.Error())
	}
	filename := "job_file_" + jobName + ".txt"
	_, err = writeInFile(filename, string(statusString))
	if err != nil {
		apglog.Error("Error writing file: " + err.Error())
		return err
	}
	err = api.PostSensorData(jobName, filename)
	if err != nil {
		apglog.Error("Uploading did not work!" + err.Error())
		return err
	}
	err = os.Remove(filename)
	if err != nil {
		apglog.Error("Error removing file: " + err.Error())
		return err
	}
	return nil
}

func UploadTestFile(jobName string, fileText string) error {
	filename := "job_file_" + jobName + ".txt"
	_, err := writeInFile(filename, fileText)
	if err != nil {
		apglog.Error("Error writing file: " + err.Error())
		return err
	}
	err = api.PostSensorData(jobName, filename)
	if err != nil {
		apglog.Error("Uploading did not work!" + err.Error())
		return err
	}
	err = os.Remove(filename)
	if err != nil {
		apglog.Error("Error removing file: " + err.Error())
		return err
	}
	return nil
}
