package cli

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
)

func SetRaucSystemOkay() error {
	out, err := exec.Command("rauc", "status", "mark-good").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err
	}
	if strings.Contains(string(out), "rauc status: marked slot rootfs.0 as good") ||
		strings.Contains(string(out), "rauc status: marked slot rootfs.1 as good") {
		return nil
	} else {
		apglog.Debug("unexpected rauc-response: \"" + string(out) + "\" Pretend everything was okay.")
		return nil
	}
}

func GetRaucStatus() (string, error) {
	out, err := exec.Command("rauc", "status", "--detailed").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err.Error(), err
	}
	return string(out), nil
}

func GetFullNetworkStatus() (string, error) {
	lteOut, err := exec.Command("nmcli", "connection", "show", "\"congstar\"").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err.Error(), err
	}
	lteOut2 := "LTE:\n" + string(lteOut) + "\n"
	ethOut, err := exec.Command("nmcli", "connection", "show", "\"Wired connection 1\"").Output()
	if err != nil {
		apglog.Error(err.Error())
		return lteOut2 + err.Error(), err
	}
	ethOut2 := "Ethernet:\n" + string(ethOut) + "\n"
	// TODO: implement wifi status
	wifiOut2 := "WiFi:\n--not_yet_implemented--\n"
	return lteOut2 + ethOut2 + wifiOut2, nil
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
