package jobs

// todo make this a struct, so we can use members
// fixme: potentially unsafe file path handling when dealing with variables

import (
	"encoding/json"
	"os"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/files"
)

// todo: this should go into the configuration
const tmpStorage = "/tmp/job_files"
const bigStorage = "/data/discosat-config/job_files"

func GetDefaultSensorStatus(app *apogee.App) (api.SensorStatus, error) {
	gpsData := app.GpsService.GetData()

	cumulativeErr := error(nil)
	status := api.SensorStatus{}
	status.StatusTime = time.Now().Unix()
	status.LocationLat = gpsData.Lat
	status.LocationLon = gpsData.Lon
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

func PushStatus(app *apogee.App) error {
	newStatus, _ := GetDefaultSensorStatus(app)
	return api.PutSensorUpdate(newStatus)
}

func ReportFullStatus(jobName string, app *apogee.App) error {
	sensorName := app.SensorName()
	newStatus, _ := GetDefaultSensorStatus(app)
	statusString, err := json.Marshal(newStatus)
	if err != nil {
		apglog.Info("Error encoding the default-status: " + err.Error())
	}
	raucStatus := app.OtaService.SlotStatiString()
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

func GetLogs(jobName string, app *apogee.App, serviceName string) error {
	sensorName := app.SensorName()

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
