package net

import (
	"github.com/Wifx/gonetworkmanager/v2"
	"github.com/godbus/dbus/v5"
)

// This maps to the NetworkManager connection.type
// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/networking_guide/sec-configuring_ip_networking_with_nmcli#sec-Understanding_the_nmcli_Options
type NetworkInterfaceType string

const (
	Ethernet NetworkInterfaceType = "802-3-ethernet"
	WiFi     NetworkInterfaceType = "802-11-wireless"
	GSM      NetworkInterfaceType = "gsm"
)

type NetworkService interface {
	GetConnectionStateByType(NetworkInterfaceType) (gonetworkmanager.NmActiveConnectionState, error)
	GetConnectionStateStr(NetworkInterfaceType) (string, error)
	IsNetworkTypeActive(NetworkInterfaceType) (bool, error)
	HasConnectivity() bool
	Shutdown()
	ConnectToWiFi(targetSSID string, psk string)

	// Private
	initialize() error
}

func NewService(conn *dbus.Conn) (NetworkService, error) {
	// todo: First try dbus, if fails cli
	e := &networkDbusService{conn: conn}
	return e, e.initialize()
}
