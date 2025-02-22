package cli

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/LeoCommon/client/pkg/log"
	"go.uber.org/zap"
)

func GetDiskStatus() (string, error) {
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		log.Error(err.Error())
		return err.Error(), err
	}
	return string(out), nil
}

func GetTimingStatus() (string, error) {
	trackingOut, err := exec.Command("chronyc", "tracking").Output()
	if err != nil {
		log.Error(err.Error())
		return err.Error(), err
	}
	sourcesOut, err := exec.Command("chronyc", "sources").Output()
	if err != nil {
		log.Error(err.Error())
		return string(trackingOut) + "\n" + err.Error(), err
	}
	return string(trackingOut) + "\n" + string(sourcesOut), nil
}

func GetSystemdStatus() (string, error) {
	out, err := exec.Command("systemctl", "status").Output()
	if err != nil {
		log.Error(err.Error())
		return err.Error(), err
	}
	return string(out), nil
}

func GetTemperature() (float64, error) {
	out, err := exec.Command("cat", "/sys/class/thermal/thermal_zone0/temp").Output()
	if err != nil {
		log.Error(err.Error())
		return 999.998, err
	}
	tempString := strings.Replace(string(out), "\n", "", -1)
	convToInt, err := strconv.Atoi(tempString)
	if err != nil {
		log.Error(err.Error())
		return 999.997, err
	}
	temperature := float64(convToInt) / 1000.0
	return temperature, nil
}

func GetServiceLogs(serviceName string) (string, error) {
	output := ""
	for i := 9; i >= 0; i-- {
		command := "journalctl -u" + serviceName + " -b " + strconv.Itoa(i) + " --no-pager"
		out, err := exec.Command("journalctl", "-u", serviceName, "-b", strconv.Itoa(i), "--no-pager").Output()
		if err != nil {
			log.Error("error reading journalctl", zap.String("unit", serviceName), zap.String("command", command))
		}
		output += "\n" + command + "\n"
		output += string(out)
	}
	return output, nil
}

// Soft reboot runs 10 seconds after invocation
const SoftRebootDelaySec = 10

// This command prepares a soft reboot
// Execution of this call with .run() needs the proper permissions => see polkit.rules
func PrepareSoftReboot() *exec.Cmd {
	return exec.Command("systemd-run", fmt.Sprintf("--on-active=%ds", SoftRebootDelaySec), "systemctl", "reboot")
}
