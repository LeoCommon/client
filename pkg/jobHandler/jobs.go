package jobHandler

import (
	"encoding/json"
	"os"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/files"
)

const tmpStorage = "/tmp/job_files"
const bigStorage = "/data/discosat-config/job_files"

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

func ReportFullStatus(jobName string, sensorName string) error {
	newStatus, _ := GetDefaultSensorStatus()
	statusString, err := json.Marshal(newStatus)
	if err != nil {
		apglog.Info("Error encoding the default-status: " + err.Error())
	}
	raucStatus := "UNIMPLEMENTED" // #fixme cli.GetRaucStatus() needs porting to dbus
	networkStatus, _ := cli.GetFullNetworkStatus()
	diskStatus, _ := cli.GetDiskStatus()
	timingStatus, _ := cli.GetTimingStatus()
	systemctlStatus, _ := cli.GetSystemdStatus()
	totalStatus := sensorName + "\n\n" + string(statusString) + "\n\nRauc-Status:\n" + raucStatus + "\nNetwork-Status:\n" + networkStatus +
		"\nDisk-Status:\n" + diskStatus + "\nTiming-Status:\n" + timingStatus + "\nSystemctl-Status:\n" + systemctlStatus
	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := tmpStorage + "/" + filename
	_, err = files.WriteInFile(filePath, totalStatus)
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

func UploadTestFile(jobName string, sensorName string, fileText string) error {
	filename := "job_" + jobName + "_sensor" + sensorName + ".txt"
	filePath := tmpStorage + "/" + filename
	_, err := files.WriteInFile(filePath, fileText)
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

func GetLogs(jobName string, sensorName string, serviceName string) error {
	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := tmpStorage + "/" + filename
	serviceLogs, err := cli.GetServiceLogs(serviceName)
	if err != nil {
		apglog.Error("Error reading serviceLogs: " + err.Error())
		serviceLogs = serviceLogs + err.Error()
	}
	_, err = files.WriteInFile(filePath, serviceLogs)
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
