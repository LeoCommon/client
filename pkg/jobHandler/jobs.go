package jobHandler

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
)

const fileStorage = "/tmp/job_files"

func writeInFile(filePath string, text string) (os.File, error) {
	_, err := os.Stat(fileStorage)
	if os.IsNotExist(err) {
		err := os.Mkdir(fileStorage, 0755)
		if err != nil {
			return os.File{}, err
		}
	}
	f, err := os.Create(filePath)
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

func GetDefaultSensorStatus() (api.SensorStatus, error) {
	cumulativeErr := error(nil)
	status := api.SensorStatus{}
	status.StatusTime = time.Now().Unix()
	status.LocationLat = "123.456"
	status.LocationLon = "123.456"
	status.OsVersion = "0.1b"
	myTemp, err := cli.GetTemperature()
	if err != nil {
		cumulativeErr = err
	}
	status.TemperatureCelsius = myTemp
	lteStatus, wifiStatus, ethStatus, err := cli.GetNetworksStatus()
	if err != nil {
		cumulativeErr = err
	}
	status.LTE = lteStatus
	status.WiFi = wifiStatus
	status.Ethernet = ethStatus
	if cumulativeErr != nil {
		return status, cumulativeErr
	}
	return status, nil
}

func PushStatus() error {
	newStatus, _ := GetDefaultSensorStatus()
	err := api.PutSensorUpdate(newStatus)
	return err
}

func ReportFullStatus(jobName string) error {
	newStatus, _ := GetDefaultSensorStatus()
	statusString, err := json.Marshal(newStatus)
	if err != nil {
		apglog.Info("Error encoding the default-status: " + err.Error())
	}
	raucStatus, _ := cli.GetRaucStatus()
	networkStatus, _ := cli.GetFullNetworkStatus()
	diskStatus, _ := cli.GetDiskStatus()
	timingStatus, _ := cli.GetTimingStatus()
	systemctlStatus, _ := cli.GetSystemdStatus()
	totalStatus := string(statusString) + "\nRauc-Status:\n" + raucStatus + "\nNetwork-Status:\n" + networkStatus +
		"\nDisk-Status:\n" + diskStatus + "\nTiming-Status:\n" + timingStatus + "\nSystemctl-Status:\n" + systemctlStatus
	filename := "job_file_" + jobName + ".txt"
	filePath := fileStorage + "/" + filename
	_, err = writeInFile(filePath, totalStatus)
	if err != nil {
		apglog.Error("Error writing file: " + err.Error())
		return err
	}
	err = api.PostSensorData(jobName, filename, filePath)
	if err != nil {
		apglog.Error("Uploading did not work!" + err.Error())
		return err
	}
	err = os.Remove(filePath)
	if err != nil {
		apglog.Error("Error removing file: " + err.Error())
		return err
	}
	return nil
}

func UploadTestFile(jobName string, fileText string) error {
	filename := "job_file_" + jobName + ".txt"
	filePath := fileStorage + "/" + filename
	_, err := writeInFile(filePath, fileText)
	if err != nil {
		apglog.Error("Error writing file: " + err.Error())
		return err
	}
	err = api.PostSensorData(jobName, filename, filePath)
	if err != nil {
		apglog.Error("Uploading did not work!" + err.Error())
		return err
	}
	err = os.Remove(filePath)
	if err != nil {
		apglog.Error("Error removing file: " + err.Error())
		return err
	}
	return nil
}
