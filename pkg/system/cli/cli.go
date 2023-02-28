package cli

import (
	"os/exec"
	"strconv"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"go.uber.org/zap"
)

func GetDiskStatus() (string, error) {
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err.Error(), err
	}
	return string(out), nil
}

func GetTimingStatus() (string, error) {
	trackingOut, err := exec.Command("chronyc", "tracking").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err.Error(), err
	}
	sourcesOut, err := exec.Command("chronyc", "sources").Output()
	if err != nil {
		apglog.Error(err.Error())
		return string(trackingOut) + "\n" + err.Error(), err
	}
	return string(trackingOut) + "\n" + string(sourcesOut), nil
}

func GetSystemdStatus() (string, error) {
	out, err := exec.Command("systemctl", "status").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err.Error(), err
	}
	return string(out), nil
}

func GetTemperature() (float64, error) {
	out, err := exec.Command("cat", "/sys/class/thermal/thermal_zone0/temp").Output()
	if err != nil {
		apglog.Error(err.Error())
		return 999.998, err
	}
	tempString := strings.Replace(string(out), "\n", "", -1)
	convToInt, err := strconv.Atoi(tempString)
	if err != nil {
		apglog.Error(err.Error())
		return 999.997, err
	}
	temperature := float64(convToInt) / 1000.0
	return temperature, nil
}

func GetServiceLogs(serviceName string) (string, error) {
	out, err := exec.Command("journalctl", "-u", serviceName, "-b", "--no-pager").Output()
	if err != nil {
		apglog.Error("error reading journalctl", zap.String("unit", serviceName))
		return "", err
	}
	return string(out), nil
}

// This command prepares a soft reboot
// Execution of this call with .run() needs the proper permissions => see polkit.rules
func PrepareSoftReboot() *exec.Cmd {
	return exec.Command("systemctl", "reboot")
}
