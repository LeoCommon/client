package net

import (
	"net/netip"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
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
	Network netip.Prefix
	Gateway netip.Addr
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
type networkConfig struct {
	v4         *V4Config
	v6         *V6Config
	dnsServers []string
	device     NetworkDevice
	settings   ConnectionSettings
}

func NewNetworkConfig() networkConfig {
	conf := networkConfig{}

	return conf
}

func NewWiredNetworkConfig() networkConfig {
	conf := networkConfig{}
	conf.device.Type = Ethernet

	return conf
}

func (nc *networkConfig) WithName(name string) *networkConfig {
	nc.settings.Name = name
	return nc
}

func (nc *networkConfig) WithUUID(uuidstr string) *networkConfig {
	u, err := uuid.Parse(uuidstr)
	if err != nil {
		apglog.Error("invalid uuid, ignoring", zap.Error(err), zap.String("uuid", uuidstr))
		return nc
	}

	nc.settings.UUID = &u
	return nc
}

func (nc *networkConfig) WithAutoconnect(acSetting *AutoConnectSettings) *networkConfig {
	nc.settings.AutoConnect = acSetting
	return nc
}

func (nc *networkConfig) WithDeviceName(name string) *networkConfig {
	nc.device.Name = name
	return nc
}

func (nc *networkConfig) WithV4Static(net *V4Config) *networkConfig {
	nc.v4 = net
	return nc
}

func (nc *networkConfig) WithV6Static(net *V6Config) *networkConfig {
	nc.v6 = net
	return nc
}

func (nc *networkConfig) WithV4Automatic() *networkConfig {
	nc.v4 = &V4Config{}
	return nc
}

func (nc *networkConfig) WithV6Automatic() *networkConfig {
	nc.v6 = &V6Config{}
	return nc
}

func (nc *networkConfig) WithV6AddrModeEUI64() *networkConfig {
	nc.v6.eui64 = true
	return nc
}

func (nc *networkConfig) WithCustomDNS(customDNS []string) *networkConfig {
	nc.dnsServers = customDNS
	return nc
}

// Sets the device type for the network configuration
// Please only use this if you know what you are doing
func (nc *networkConfig) WithDeviceType(t NetworkInterfaceType) *networkConfig {
	nc.device.Type = t
	return nc
}

type wirelessNetworkConfig struct {
	networkConfig
	ssid string
	psk  string // Preferably in a network manager encrypted format
}

func NewWirelessNetworkConfig(SSID string, PSK string) wirelessNetworkConfig {
	wconf := wirelessNetworkConfig{ssid: SSID, psk: PSK}
	wconf.device.Type = WiFi

	return wconf
}

func NewWirelessConfigFromNetworkConfig(SSID string, PSK string, networkconf networkConfig) wirelessNetworkConfig {
	conf := NewWirelessNetworkConfig(SSID, PSK)
	conf.networkConfig = networkconf

	// Override the type from the provided networkConfig
	conf.device.Type = WiFi
	return conf
}

type gsmNetworkConfig struct {
	networkConfig
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

	EnforceNetworkPriority() error

	// Create a connection
	CreateConnection(config interface{}) error

	// Private
	initialize() error
}

func NewService(conn *dbus.Conn) (NetworkService, error) {
	// todo: First try dbus, if fails cli
	e := &networkDbusService{conn: conn}
	return e, e.initialize()
}
