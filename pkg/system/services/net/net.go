package net

import (
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
	Invalid  NetworkInterfaceType = "invalid"
)

type Static struct {
	Address string
	Prefix  byte
	Gateway string
}

type V4Config struct {
	Static *Static
}
type V6Config struct {
	Static *Static
	eui64  bool
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
	v4         *V4Config
	v6         *V6Config
	dnsServers []string
	device     NetworkDevice
	settings   ConnectionSettings
}

func (nc *NetworkConfig) WithName(name string) *NetworkConfig {
	nc.settings.Name = name
	return nc
}

func (nc *NetworkConfig) WithUUID(uuid *uuid.UUID) *NetworkConfig {
	nc.settings.UUID = uuid
	return nc
}

func (nc *NetworkConfig) WithAutoconnect(acSetting *AutoConnectSettings) *NetworkConfig {
	nc.settings.AutoConnect = acSetting
	return nc
}

func (nc *NetworkConfig) WithDeviceName(name string) *NetworkConfig {
	nc.device.Name = name
	return nc
}

func (nc *NetworkConfig) WithV4Static(net V4Config) *NetworkConfig {
	nc.v4 = &net
	return nc
}

func (nc *NetworkConfig) WithV6Static(net V6Config) *NetworkConfig {
	nc.v6 = &net
	return nc
}

func (nc *NetworkConfig) WithV4Automatic() *NetworkConfig {
	nc.v4 = &V4Config{}
	return nc
}

func (nc *NetworkConfig) WithV6Automatic() *NetworkConfig {
	nc.v6 = &V6Config{}
	return nc
}

func (nc *NetworkConfig) WithV6AddrModeEUI64() *NetworkConfig {
	nc.v6.eui64 = true
	return nc
}

func (nc *NetworkConfig) WithCustomDNS(customDNS []string) *NetworkConfig {
	nc.dnsServers = customDNS
	return nc
}

// Sets the device type for the network configuration
// Please only use this if you know what you are doing
func (nc *NetworkConfig) WithDeviceType(t NetworkInterfaceType) *NetworkConfig {
	nc.device.Type = t
	return nc
}

type wirelessNetworkConfig struct {
	NetworkConfig
	ssid string
	psk  string // Preferably in a network manager encrypted format
}

func NewWirelessNetworkConfig(SSID string, PSK string) wirelessNetworkConfig {
	wconf := wirelessNetworkConfig{ssid: SSID, psk: PSK}
	wconf.device.Type = WiFi

	return wconf
}

type gsmNetworkConfig struct {
	NetworkConfig
	APN      string
	Username string
	Password string
}

func NewGSMNetworkConfig(APN string, Username string, Password string) gsmNetworkConfig {
	conf := gsmNetworkConfig{APN: APN, Username: Username, Password: Password}
	conf.device.Type = GSM

	return conf
}

type NetworkService interface {
	GetConnectionStateStr(NetworkInterfaceType) (string, error)
	IsNetworkTypeActive(NetworkInterfaceType) (bool, error)
	SetDeviceStateByType(NetworkInterfaceType, bool) error
	HasConnectivity() bool
	Shutdown()

	// Testing
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
