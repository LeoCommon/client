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
	"syscall"
	"time"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/constants"
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
	status.OsVersion = "1.0c"
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

func PushStatus(app *client.App) error {
	newStatus, _ := GetDefaultSensorStatus(app)
	return app.Api.PutSensorUpdate(newStatus)
}

// GetFullNetworkStatus #fixme this should return more data but its sufficient for now
func GetFullNetworkStatus(app *client.App) string {

	// #fixme this is closest to the original, but ideally we should get all available / active ones
	connections := map[net.NetworkInterfaceType]string{
		net.Ethernet: "eth",
		net.WiFi:     "wifi",
		net.GSM:      "gsm",
	}

	// iterate over all connection types
	outputStr := ""
	for conType, name := range connections {
		state, err := app.NetworkService.GetConnectionStateStr(conType)
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

func ReportFullStatus(ctx context.Context, jobName string, app *client.App) error {
	sensorName := app.Conf.Api().SensorName()
	newStatus, _ := GetDefaultSensorStatus(app)
	statusString, err := json.Marshal(newStatus)
	if err != nil {
		log.Info("Error encoding the default-status: " + err.Error())
	}
	raucStatus := app.OtaService.SlotStatiString()
	networkStatus := GetFullNetworkStatus(app)
	diskStatus, _ := cli.GetDiskStatus()
	timingStatus, _ := cli.GetTimingStatus()
	systemctlStatus, _ := cli.GetSystemdStatus()
	totalStatus := sensorName + "\n\n" + string(statusString) + "\n\nRauc-Status:\n" + raucStatus + "\nNetwork-Status:\n" + networkStatus +
		"\nDisk-Status:\n" + diskStatus + "\nTiming-Status:\n" + timingStatus + "\nSystemctl-Status:\n" + systemctlStatus
	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := filepath.Join(app.Conf.Jobs().TempPath(), filename)
	err = file.WriteTo(filePath, totalStatus)
	if err != nil {
		log.Error("Error writing file: " + err.Error())
		return err
	}
	err = app.Api.PostSensorData(ctx, jobName, filename, filePath)
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

func GetLogs(ctx context.Context, job api.FixedJob, app *client.App) error {
	serviceName := job.Arguments["service"]
	if len(serviceName) == 0 {
		serviceName = constants.ClientServiceName
	}

	jobName := job.Name
	sensorName := app.Conf.Api().SensorName()

	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := filepath.Join(app.Conf.Jobs().TempPath(), filename)

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
	err = app.Api.PostSensorData(ctx, jobName, filename, filePath)
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

// ForceReset flushes files and then issues a kernel level reboot, bypassing systemd
func ForceReset() error {
	syscall.Sync()
	err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)

	if err != nil {
		log.Error("could not reset system!", zap.Error(err))
	}

	return err
}

func RebootSensor(job api.FixedJob, app *client.App) error {
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
