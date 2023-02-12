package cli

import (
	"os/exec"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"go.uber.org/zap"
)

func GetFullNetworkStatus(app *apogee.App) (string, error) {
	apglog.Error("STUB: GetFullNetworkStatus")
	return "", nil
}

func activateGenericNetwork(newActiveState bool, networkName string) error {
	apglog.Error("STUB: activateGenericNetwork")
	return nil
}

func ActivateNetworks(ethActive bool, wifiActive bool, gsmActive bool, app *apogee.App) error {
	apglog.Error("STUB: ActivateNetworks")
	return nil
}

func ReloadNetworks() error {
	_, err := exec.Command("nmcli", "connection", "reload").Output()
	if err != nil {
		apglog.Error("Error reloading network connections ", zap.Error(err))
		return err
	}
	apglog.Debug("network connection reloaded")
	return nil
}

func SetNetworkConfigFileRights(fileName string) error {
	_, err := exec.Command("chmod", "0600", fileName).Output()
	if err != nil {
		apglog.Error("Error setting network-config file rights", zap.Error(err))
		return err
	}
	return nil
}
