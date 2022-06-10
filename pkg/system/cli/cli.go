package cli

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
)

func GetFullNetworkStatus() (string, error) {
	lteName := "congstar"
	lteOut, err := exec.Command("nmcli", "connection", "show", lteName).Output()
	lteOut2 := "LTE:\n" + string(lteOut) + "\n"
	if err != nil {
		apglog.Error(err.Error())
		return lteOut2 + "Error: " + err.Error() + "\n", err
	}
	ethName := "Wired connection 1"
	ethOut, err := exec.Command("nmcli", "connection", "show", ethName).Output()
	ethOut2 := "Ethernet:\n" + string(ethOut) + "\n"
	if err != nil {
		apglog.Error(err.Error())
		return lteOut2 + ethOut2 + "Error: " + err.Error() + "\n", err
	}
	// TODO: implement wifi status
	wifiOut2 := "WiFi:\n--not_yet_implemented--\n"
	return lteOut2 + ethOut2 + wifiOut2 + "\n", nil
}

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

func GetNetworksStatus() (string, string, string, error) {

	allConnections, err := exec.Command("nmcli", "connection", "show").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err.Error(), err.Error(), err.Error(), err
	}
	cumulativeError := errors.New("")
	cumulativeError = nil
	// check LTE
	lteName := "congstar"
	lteResult := "noConfig"
	if strings.Contains(string(allConnections), lteName) {
		lteResult = "inactive"
		lteOut, err := exec.Command("nmcli", "-f", "GENERAL.STATE", "con", "show", lteName).Output()
		if err != nil {
			apglog.Error(err.Error())
			lteResult = err.Error()
			cumulativeError = err
		} else if strings.Contains(string(lteOut), "activated") {
			lteResult = "activated"
		}
	}
	// check WiFi
	wifiResult := "--not_yet_implemented--"
	// check Ethernet
	ethName := "Wired connection 1"
	ethResult := "noConfig"
	if strings.Contains(string(allConnections), ethName) {
		ethResult = "inactive"
		ethOut, err := exec.Command("nmcli", "-f", "GENERAL.STATE", "con", "show", ethName).Output()
		if err != nil {
			apglog.Error(err.Error())
			ethResult = err.Error()
			cumulativeError = err
		} else if strings.Contains(string(ethOut), "activated") {
			ethResult = "activated"
		}
	}
	return lteResult, wifiResult, ethResult, cumulativeError
}

func GetServiceLogs(serviceName string) (string, error) {
	// the logs are deleted on every reboot, so using '-b' or not doesn't make any difference
	out, err := exec.Command("journalctl", "-u", serviceName, "-b", "--no-pager").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err.Error(), err
	}
	return string(out), nil
}
