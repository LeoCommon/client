package jobs

// todo make this a struct, so we can use members
// fixme: potentially unsafe file path handling when dealing with variables

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/constants"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/schema"
	"disco.cs.uni-kl.de/apogee/pkg/file"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"
)

var (
	ErrJobDisabled = errors.New("this job type is disabled")
)

func GetDefaultSensorStatus(app *client.App) (api.SensorStatus, error) {
	gpsData := app.GNSSService.GetData()

	cumulativeErr := error(nil)
	status := api.SensorStatus{}
	status.StatusTime = time.Now().Unix()
	status.LocationLat = gpsData.Lat
	status.LocationLon = gpsData.Lon
	status.OsVersion = "1.0d"
	myTemp, err := cli.GetTemperature()
	if err != nil {
		cumulativeErr = err
	}
	status.TemperatureCelsius = myTemp

	// #todo check and improve error handling
	if app.NetworkService != nil {
		gsmStatus, _ := app.NetworkService.GetConnectionStateStr(net.GSM)
		wifiStatus, _ := app.NetworkService.GetConnectionStateStr(net.WiFi)
		ethStatus, _ := app.NetworkService.GetConnectionStateStr(net.Ethernet)

		status.LTE = gsmStatus
		status.WiFi = wifiStatus
		status.Ethernet = ethStatus
	}

	if cumulativeErr != nil {
		return status, cumulativeErr
	}
	return status, nil
}

func PushStatus(jp *schema.JobParameters) error {
	newStatus, _ := GetDefaultSensorStatus(jp.App)
	return jp.App.Api.PutSensorUpdate(newStatus)
}

// GetFullNetworkStatus #fixme this should return more data but its sufficient for now
func GetFullNetworkStatus(jp *schema.JobParameters) string {

	// #fixme this is closest to the original, but ideally we should get all available / active ones
	connections := map[net.NetworkInterfaceType]string{
		net.Ethernet: "eth",
		net.WiFi:     "wifi",
		net.GSM:      "gsm",
	}

	// iterate over all connection types
	outputStr := ""
	for conType, name := range connections {
		state, err := jp.App.NetworkService.GetConnectionStateStr(conType)
		if err != nil {
			_, ok := err.(*net.ConnectionNotAvailable)
			if ok {
				log.Info("skipping unavailable connection type", zap.String("type", string(conType)))
				continue
			}

			// If there was a different error, include this in our output
			state = err.Error()
		}

		outputStr += fmt.Sprintf("%s:\n", name)
		outputStr += fmt.Sprintf("\tstate: %v\n", state)
	}

	return outputStr
}

func ReportFullStatus(ctx context.Context, jobName string, jp *schema.JobParameters) error {
	sensorName := jp.App.Conf.SensorName()
	newStatus, _ := GetDefaultSensorStatus(jp.App)
	statusString, err := json.Marshal(newStatus)
	if err != nil {
		log.Info("Error encoding the default-status: " + err.Error())
	}
	raucStatus := jp.App.OtaService.SlotStatiString()
	networkStatus := GetFullNetworkStatus(jp)
	diskStatus, _ := cli.GetDiskStatus()
	timingStatus, _ := cli.GetTimingStatus()
	systemctlStatus, _ := cli.GetSystemdStatus()
	totalStatus := sensorName + "\n\n" + string(statusString) + "\n\nRauc-Status:\n" + raucStatus + "\nNetwork-Status:\n" + networkStatus +
		"\nDisk-Status:\n" + diskStatus + "\nTiming-Status:\n" + timingStatus + "\nSystemctl-Status:\n" + systemctlStatus
	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := filepath.Join(jp.App.Conf.JobTempPath(), filename)
	err = file.WriteTo(filePath, totalStatus)
	if err != nil {
		log.Error("Error writing file: " + err.Error())
		return err
	}
	err = jp.App.Api.PostSensorData(ctx, jobName, filename, filePath)
	if err != nil {
		log.Error("Uploading did not work!" + err.Error())
		return err
	}
	err = os.Remove(filePath)
	if err != nil {
		log.Error("Error removing file: " + err.Error())
		return err
	}
	return nil
}

func GetLogs(ctx context.Context, job api.FixedJob, jp *schema.JobParameters) error {
	serviceName := job.Arguments["service"]
	if len(serviceName) == 0 {
		serviceName = constants.ClientServiceName
	}

	jobName := job.Name
	sensorName := jp.App.Conf.SensorName()

	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := filepath.Join(jp.Config.TempDir.String(), filename)

	serviceLogs, err := cli.GetServiceLogs(serviceName)
	if err != nil {
		log.Error("Error reading serviceLogs: " + err.Error())
		serviceLogs = serviceLogs + err.Error()
	}
	err = file.WriteTo(filePath, serviceLogs)
	if err != nil {
		log.Error("Error writing file: " + err.Error())
		return err
	}
	err = jp.App.Api.PostSensorData(ctx, jobName, filename, filePath)
	if err != nil {
		log.Error("Uploading did not work!" + err.Error())
		return err
	}
	err = os.Remove(filePath)
	if err != nil {
		log.Error("Error removing file: " + err.Error())
		return err
	}
	return nil
}

func GetConfig(ctx context.Context, job api.FixedJob, jp *schema.JobParameters) error {
	configType := job.Arguments["type"]
	if len(configType) == 0 {
		configType = "all"
	}

	jobName := job.Name
	sensorName := jp.App.Conf.SensorName()

	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := filepath.Join(jp.Config.TempDir.String(), filename)

	configData := "type:" + configType + "\n"
	if configType == "shortcut" {
		// special type to avoid writing any files (broken configs). returns the config-text as an error
		configData += getConstants()
		configData += getConfigs(jp)
		return fmt.Errorf(configData)
	} else if configType == "all" {
		configData += getConstants()
		configData += getConfigs(jp)
	}

	err := file.WriteTo(filePath, configData)
	if err != nil {
		log.Error("Error writing file: " + err.Error())
		return err
	}
	err = jp.App.Api.PostSensorData(ctx, jobName, filename, filePath)
	if err != nil {
		log.Error("Uploading did not work!" + err.Error())
		return err
	}
	err = os.Remove(filePath)
	if err != nil {
		log.Error("Error removing file: " + err.Error())
		return err
	}
	return nil
}

func SetConfig(job api.FixedJob, jp *schema.JobParameters) error {
	configsMap := job.Arguments
	err := error(nil)
	for key := range configsMap {
		if key == "job_temp_path" {
			err = jp.App.Conf.SetJobTempPath(configsMap[key])
		} else if key == "job_storage_path" {
			err = jp.App.Conf.SetJobStoragePath(configsMap[key])
		} else if key == "polling_interval" {
			err = jp.App.Conf.SetPollingInterval(configsMap[key])
		}
		if err != nil {
			log.Error("Error setting config " + key + "=" + configsMap[key] + ": " + err.Error())
			return err
		}
	}

	return err
}

// ForceReset flushes files and then issues a kernel level reboot, bypassing systemd
func ForceReset() error {
	syscall.Sync()
	err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)

	if err != nil {
		log.Error("could not reset system!", zap.Error(err))
	}

	return err
}

func RebootSensor(job api.FixedJob, jp *schema.JobParameters) error {
	// #FIXME this is not properly handled, we should never reboot instantly, shut down first!
	log.Error("STUB: RebootSensor, please implement properly!")
	return fmt.Errorf("reboot not implemented at the moment")

	/*
			jobName := job.name

			// Assume everything works and send a "finished" status (later you can't send it).
			err := api.PutJobUpdate(jobName, "finished")
			if err != nil {
				log.Error("Error when contacting server before reboot-job execution", zap.Error(err))
				return err
			}

			err = cli.PrepareSoftReboot()
			if err != nil {
				log.Error("Error when performing reboot-job", zap.Error(err))
				err := api.PutJobUpdate(jobName, "failed")
				if err != nil {
					log.Error("Error during sending error in reboot-job", zap.Error(err))
					return err
				}
			}

		return nil
	*/
}

func getConstants() string {
	result := "constants\n"
	result += "constants.ClientServiceName=" + constants.ClientServiceName + "\n"
	result += "constants.RebootPendingTmpfile=" + constants.RebootPendingTmpfile + "\n"
	return result
}

func getConfigs(jp *schema.JobParameters) string {
	result := "configs\n"
	result += "jp.App.Api.GetBaseURL=" + jp.App.Api.GetBaseURL() + "\n"
	result += "jp.App.Conf.SensorName=" + jp.App.Conf.SensorName() + "\n"
	result += "jp.App.Conf.JobTempPath=" + jp.App.Conf.JobTempPath() + "\n"
	result += "jp.App.Conf.JobStoragePath=" + jp.App.Conf.JobStoragePath() + "\n"
	result += "jp.Config.PollingInterval=" + jp.Config.PollingInterval.Value().String() + "\n"
	result += "jp.Config.TempDir=" + jp.Config.TempDir.String() + "\n"
	result += "jp.Config.StorageDir=" + jp.Config.StorageDir.String() + "\n"
	result += "jp.Config.Iridium.Disabled=" + strconv.FormatBool(jp.Config.Iridium.Disabled) + "\n"
	result += "jp.Config.Network.Disabled=" + strconv.FormatBool(jp.Config.Network.Disabled) + "\n"
	return result
}
