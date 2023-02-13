package net

import (
	gonm "github.com/Wifx/gonetworkmanager/v2"
	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
)

// This maps to the NetworkManager connection.type
// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/networking_guide/sec-configuring_ip_networking_with_nmcli#sec-Understanding_the_nmcli_Options
type NetworkInterfaceType string

const (
	Ethernet NetworkInterfaceType = "802-3-ethernet"
	WiFi     NetworkInterfaceType = "802-11-wireless"
	GSM      NetworkInterfaceType = "gsm"
)

type V46Static struct {
	Address string
	Prefix  byte
	Gateway string
	DNS     []string
}
type V4Config struct {
	V46Static
	DHCP bool
}
type V6Config struct {
	V46Static
	EUI64 bool
	SLAAC bool
}

type AutoConnectSettings struct {
	State    bool
	Priority *int32
	Retries  *int32
}
type ConnectionSettings struct {
	// The name of the connection
	Name string
	// An optional UUID, will get auto-generated if not set
	UUID *uuid.UUID
	// The autoconnect settings
	AutoConnect *AutoConnectSettings
}

type NetworkDevice struct {
	Name string
	Type NetworkInterfaceType
}
type NetworkConfig struct {
	V4       *V4Config
	V6       *V6Config
	Device   NetworkDevice
	Settings ConnectionSettings
}

type WirelessNetworkConfig struct {
	NetworkConfig
	SSID string
	PSK  string // Preferably in a network manager encrypted format
}

type GSMNetworkConfig struct {
	NetworkConfig
	APN      string
	Username string // Preferably in a network manager encrypted format
	Password string
}

type NetworkService interface {
	GetConnectionStateByType(NetworkInterfaceType) (gonm.NmActiveConnectionState, error)
	GetConnectionStateStr(NetworkInterfaceType) (string, error)
	IsNetworkTypeActive(NetworkInterfaceType) (bool, error)
	HasConnectivity() bool
	Shutdown()

	// Testing
	FindWorkingConnection() (gonm.Device, error)
	EnforceNetworkPriority() error
	CreateConnection(config interface{}) error

	// Private
	initialize() error
}

func NewService(conn *dbus.Conn) (NetworkService, error) {
	// todo: First try dbus, if fails cli
	e := &networkDbusService{conn: conn}
	return e, e.initialize()
}
