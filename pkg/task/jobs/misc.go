package jobs

// todo make this a struct, so we can use members
// fixme: potentially unsafe file path handling when dealing with variables

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/files"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"
)

func GetDefaultSensorStatus(app *apogee.App) (api.SensorStatus, error) {
	gpsData := app.GpsService.GetData()

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
	gsmStatus, _ := app.NetworkService.GetConnectionStateStr(net.GSM)
	wifiStatus, _ := app.NetworkService.GetConnectionStateStr(net.WiFi)
	ethStatus, _ := app.NetworkService.GetConnectionStateStr(net.Ethernet)

	status.LTE = gsmStatus
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
	networkStatus, _ := cli.GetFullNetworkStatus(app)
	diskStatus, _ := cli.GetDiskStatus()
	timingStatus, _ := cli.GetTimingStatus()
	systemctlStatus, _ := cli.GetSystemdStatus()
	totalStatus := sensorName + "\n\n" + string(statusString) + "\n\nRauc-Status:\n" + raucStatus + "\nNetwork-Status:\n" + networkStatus +
		"\nDisk-Status:\n" + diskStatus + "\nTiming-Status:\n" + timingStatus + "\nSystemctl-Status:\n" + systemctlStatus
	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := app.Config.Client.Jobs.TempRecStorage + filename
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

func GetLogs(job api.FixedJob, app *apogee.App) error {
	serviceName := job.Arguments["service"]
	if len(serviceName) == 0 {
		serviceName = "apogee-client.service"
	}

	jobName := job.Name
	sensorName := app.SensorName()

	filename := "job_" + jobName + "_sensor_" + sensorName + ".txt"
	filePath := app.Config.Client.Jobs.TempRecStorage + filename
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

func RebootSensor(job api.FixedJob, app *apogee.App) error {
	jobName := job.Name

	// Assume everything works and send a "finished" status (later you can't send it).
	err := api.PutJobUpdate(jobName, "finished")
	if err != nil {
		apglog.Error("Error when contacting server before reboot-job execution", zap.Error(err))
		return err
	}

	err = cli.SoftReboot()
	if err != nil {
		apglog.Error("Error when performing reboot-job", zap.Error(err))
		err := api.PutJobUpdate(jobName, "failed")
		if err != nil {
			apglog.Error("Error during sending error in reboot-job", zap.Error(err))
			return err
		}
	}

	return nil
}

func SetNetworkConnectivity(job api.FixedJob, app *apogee.App) error {
	// everything on is the default state
	ethState := true
	wifiState := true
	gsmState := true
	arguments := job.Arguments
	// parse the arguments
	keys := make([]string, len(arguments))
	i := 0
	for k := range arguments {
		keys[i] = k
		i++
	}
	// go through the keys
	for i := 0; i < len(keys); i++ {
		tempKey := keys[i]
		tempValue := arguments[tempKey]
		tempKey = strings.ToLower(tempKey)
		if strings.Contains(tempKey, "eth") {
			if strings.Contains(tempValue, "off") {
				ethState = false
			}
		} else if strings.Contains(tempKey, "wifi") {
			if strings.Contains(tempValue, "off") {
				wifiState = false
			}
		} else if strings.Contains(tempKey, "gsm") {
			if strings.Contains(tempValue, "off") {
				gsmState = false
			}
		} else {
			apglog.Info("Unknown network argument: " + tempKey + ":" + tempValue)
		}
	}
	// activate the connection
	err := cli.ActivateNetworks(ethState, wifiState, gsmState, app)
	return err
}

func parseIPconfigs(inputMap map[string]string) (string, string, string, string, string, error) {
	autoconnect := "true"
	methodIPv4 := "auto"
	addressesIPv4 := ""
	gatewayIPv4 := ""
	dnsIPv4 := ""
	if val, ok := inputMap["autoconnect"]; ok {
		if strings.Contains(val, "true") || strings.Contains(val, "false") {
			autoconnect = val
		} else {
			return "", "", "", "", "", errors.New("Invalid value for parameter 'autoconnect': " + val)
		}
	} else {
		return "", "", "", "", "", errors.New("missing parameter 'autoconnect'")
	}
	if val, ok := inputMap["methodIPv4"]; ok {
		if strings.Contains(val, "auto") {
			methodIPv4 = val
		} else if strings.Contains(val, "manual") {
			methodIPv4 = val
			if val, ok := inputMap["addressesIPv4"]; ok {
				addressesIPv4 = val
			} else {
				return "", "", "", "", "", errors.New("missing parameter 'addressesIPv4'")
			}
			if val, ok := inputMap["gatewayIPv4"]; ok {
				gatewayIPv4 = val
			} else {
				return "", "", "", "", "", errors.New("missing parameter 'gatewayIPv4'")
			}
			if val, ok := inputMap["dnsIPv4"]; ok {
				dnsIPv4 = val
			} else {
				return "", "", "", "", "", errors.New("missing parameter 'dnsIPv4'")
			}
		} else {
			return "", "", "", "", "", errors.New("Invalid value for parameter 'methodIPv4': " + val)
		}
	} else {
		return "", "", "", "", "", errors.New("missing parameter 'methodIPv4'")
	}
	if val, ok := inputMap["dnsIPv4"]; ok {
		dnsIPv4 = val
	}
	return autoconnect, methodIPv4, addressesIPv4, gatewayIPv4, dnsIPv4, nil
}

func WriteWifiConfig(job api.FixedJob, app *apogee.App) error {
	ssid := "defaultDiscosatWifiName"
	psk := "defaultDiscosatWifiPw"
	// parse the wifi arguments, to ensure everything is there
	args := job.Arguments
	apglog.Debug("received arguments:", zap.Any("job.arguments", args))

	if val, ok := args["ssid"]; ok {
		ssid = val
		apglog.Debug("set ssid", zap.String("new ssid", val))
	} else {
		return errors.New("missing parameter 'ssid'")
	}
	if val, ok := args["psk"]; ok {
		psk = val
	} else {
		return errors.New("missing parameter 'psk'")
	}
	autoconnect, methodIPv4, addressesIPv4, gatewayIPv4, dnsIPv4, err := parseIPconfigs(args)
	if err != nil {
		return err
	}

	// write them into the config file
	fileName := app.Config.Client.Network.Wifi0Config
	backupFileName := fileName + ".backup"
	filecontent := "[connection]\nid=WirelessLan1\nuuid=042a1ce0-d87e-4e87-b965-ef912946f61e\ntype=wifi\n" +
		"autoconnect=" + autoconnect + "\n\n[wifi]\nssid=" + ssid + "\nmode=infrastructure\n\n[wifi-security]\nkey-mgmt=wpa-psk\npsk=" + psk + "\n\n[ipv4]\nmethod=" + methodIPv4 + "\n"
	if strings.Contains(methodIPv4, "manual") {
		filecontent = filecontent + "addresses=" + addressesIPv4 + "\ngateway=" + gatewayIPv4 + "\n"
	}
	if len(dnsIPv4) > 0 {
		filecontent = filecontent + "dns=" + dnsIPv4 + "\n"
	}
	err = files.MoveFile(fileName, backupFileName)
	if err != nil {
		return err
	}
	apglog.Debug("writing new wifi-config file", zap.String("fileName", fileName), zap.String("fileContent", filecontent))
	_, err = files.WriteInFile(fileName, filecontent)
	if err != nil {
		return err
	}
	err = cli.SetNetworkConfigFileRights(fileName)
	if err != nil {
		return err
	}
	//reload network
	err = cli.ReloadNetworks()
	if err != nil {
		return err
	}
	return nil
}

func WriteEthConfig(job api.FixedJob, app *apogee.App) error {
	// parse the arguments, to ensure everything is there
	args := job.Arguments
	autoconnect, methodIPv4, addressesIPv4, gatewayIPv4, dnsIPv4, err := parseIPconfigs(args)
	if err != nil {
		return err
	}

	// write them into the config file
	fileName := app.Config.Client.Network.Eth0Config
	backupFileName := fileName + ".backup"
	filecontent := "[connection]\nid=WiredConnection1\nuuid=4b7cecdd-63e2-39b5-a17e-0eb09c240adf\ntype=ethernet\n" +
		"autoconnect=" + autoconnect + "\n\n[ipv4]\nmethod=" + methodIPv4 + "\n"
	if strings.Contains(methodIPv4, "manual") {
		filecontent = filecontent + "addresses=" + addressesIPv4 + "\ngateway=" + gatewayIPv4 + "\n"
	}
	if len(dnsIPv4) > 0 {
		filecontent = filecontent + "dns=" + dnsIPv4 + "\n"
	}
	err = files.MoveFile(fileName, backupFileName)
	if err != nil {
		return err
	}
	_, err = files.WriteInFile(fileName, filecontent)
	if err != nil {
		return err
	}
	err = cli.SetNetworkConfigFileRights(fileName)
	if err != nil {
		return err
	}
	//reload network
	err = cli.ReloadNetworks() //not sure if this works reliably
	if err != nil {
		return err
	}
	return nil
}
