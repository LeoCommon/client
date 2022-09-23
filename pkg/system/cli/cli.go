package cli

import (
	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"go.uber.org/zap"
	"os/exec"
	"strconv"
	"strings"
)

func GetFullNetworkStatus(app *apogee.App) (string, error) {
	lteName := app.Config.Client.Network.Gsm0Name
	lteOut, err := exec.Command("nmcli", "connection", "show", lteName).Output()
	lteOut2 := "LTE:\n" + string(lteOut) + "\n"
	if err != nil {
		apglog.Error(err.Error())
		return lteOut2 + "LTE-Error: " + err.Error() + "\n", err
	}
	ethName := app.Config.Client.Network.Eth0Name
	ethOut, err := exec.Command("nmcli", "connection", "show", ethName).Output()
	ethOut2 := "Ethernet:\n" + string(ethOut) + "\n"
	if err != nil {
		apglog.Error(err.Error())
		return lteOut2 + ethOut2 + "Ethernet-Error: " + err.Error() + "\n", err
	}
	wifiName := app.Config.Client.Network.Wifi0Name
	wifiOut, err := exec.Command("nmcli", "connection", "show", wifiName).Output()
	wifiOut2 := "WiFi:\n" + string(wifiOut) + "\n"
	if err != nil {
		apglog.Error(err.Error())
		return lteOut2 + ethOut2 + wifiOut2 + "WiFi-Error: " + err.Error() + "\n", err
	}
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

func getGenericNetworkStatus(networkName string) (string, error) {
	allConnections, err := exec.Command("nmcli", "connection", "show").Output()
	if err != nil {
		apglog.Error(err.Error())
		return "", err
	}
	result := "noConfig"
	if strings.Contains(string(allConnections), networkName) {
		result = "inactive"
		nwOut, err := exec.Command("nmcli", "-f", "GENERAL.STATE", "con", "show", networkName).Output()
		if err != nil {
			apglog.Error(err.Error())
			return err.Error(), err
		} else if strings.Contains(string(nwOut), "activated") {
			result = "activated"
		}
	}
	return result, nil
}

func GetNetworksStatus(app *apogee.App) (string, string, string, error) {
	// check LTE
	lteName := app.Config.Client.Network.Gsm0Name
	lteResult, lteErr := getGenericNetworkStatus(lteName)
	// check WiFi
	wifiName := app.Config.Client.Network.Wifi0Name
	wifiResult, wifiErr := getGenericNetworkStatus(wifiName)
	// check Ethernet
	ethName := app.Config.Client.Network.Eth0Name
	ethResult, ethErr := getGenericNetworkStatus(ethName)

	if lteErr != nil {
		return lteResult, wifiResult, ethResult, lteErr
	}
	if wifiErr != nil {
		return lteResult, wifiResult, ethResult, wifiErr
	}
	if ethErr != nil {
		return lteResult, wifiResult, ethResult, ethErr
	}

	return lteResult, wifiResult, ethResult, nil
}

func activateGenericNetwork(newActiveState bool, networkName string) error {
	newState := "down"
	if newActiveState {
		newState = "up"
	}
	resp, err := exec.Command("nmcli", "connection", newState, networkName).Output()
	if strings.Contains(string(resp), "successfully") || strings.Contains(string(resp), "no active connection provided") {
		apglog.Debug("network connection '"+networkName+"' successful in new state", zap.String("new state", newState))
	}
	if err != nil {
		if strings.Contains(err.Error(), "exit status 4") {
			apglog.Error("Error activating '"+networkName+"': connection not available", zap.Error(err))
			return err
		} else if strings.Contains(err.Error(), "exit status 10") {
			apglog.Info("Expected error when deactivating '"+networkName+"': connection can not be in another state", zap.Error(err))
		} else {
			apglog.Error("Error toggling '"+networkName+"'", zap.Error(err))
			return err
		}
	}
	return nil
}

func ActivateNetworks(ethActive bool, wifiActive bool, gsmActive bool, app *apogee.App) error {
	//TODO: switch to D-Bus for the interaction
	ethErr := activateGenericNetwork(ethActive, app.Config.Client.Network.Eth0Name)
	wifiErr := activateGenericNetwork(wifiActive, app.Config.Client.Network.Wifi0Name)
	gsmErr := activateGenericNetwork(gsmActive, app.Config.Client.Network.Gsm0Name)

	if ethErr != nil {
		return ethErr
	}
	if wifiErr != nil {
		return wifiErr
	}
	if gsmErr != nil {
		return gsmErr
	}
	return nil

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

func RebootSystem() error {
	_, err := exec.Command("reboot").Output()
	if err != nil {
		apglog.Error(err.Error())
		return err
	}
	return nil
}
