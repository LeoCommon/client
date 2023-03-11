package net

import (
	"net/netip"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/systemd"
	"disco.cs.uni-kl.de/apogee/pkg/systemd/dbuscon"
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
	Priority *int32
	Retries  *int32
	State    bool
}

type ConnectionSettings struct {
	// An optional uuid, will get auto-generated if not set
	uuid *uuid.UUID
	// The autoconnect settings
	autoConnect *AutoConnectSettings
	// The name of the connection
	name string
}

type NetworkDevice struct {
	name string
	Type NetworkInterfaceType
}
type NetworkConfig struct {
	v4         *V4Config
	v6         *V6Config
	device     NetworkDevice
	settings   ConnectionSettings
	dnsServers []string
}

func NewNetworkConfig() NetworkConfig {
	conf := NetworkConfig{}

	return conf
}

func NewWiredNetworkConfig() NetworkConfig {
	conf := NetworkConfig{}
	conf.device.Type = Ethernet

	return conf
}

func (nc *NetworkConfig) WithName(name string) *NetworkConfig {
	nc.settings.name = name
	return nc
}

func (nc *NetworkConfig) WithUUID(uuidstr string) *NetworkConfig {
	u, err := uuid.Parse(uuidstr)
	if err != nil {
		log.Error("invalid uuid, ignoring", zap.Error(err), zap.String("uuid", uuidstr))
		return nc
	}

	nc.settings.uuid = &u
	return nc
}

func (nc *NetworkConfig) WithAutoconnect(acSetting *AutoConnectSettings) *NetworkConfig {
	nc.settings.autoConnect = acSetting
	return nc
}

func (nc *NetworkConfig) WithDeviceName(name string) *NetworkConfig {
	nc.device.name = name
	return nc
}

func (nc *NetworkConfig) WithV4Static(net *V4Config) *NetworkConfig {
	nc.v4 = net
	return nc
}

func (nc *NetworkConfig) WithV6Static(net *V6Config) *NetworkConfig {
	nc.v6 = net
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

// WithDeviceType Sets the device type for the network configuration
// Please only use this if you know what you are doing
func (nc *NetworkConfig) WithDeviceType(t NetworkInterfaceType) *NetworkConfig {
	nc.device.Type = t
	return nc
}

type WirelessNetworkConfig struct {
	ssid string
	psk  string
	NetworkConfig
}

func NewWirelessNetworkConfig(SSID string, PSK string) WirelessNetworkConfig {
	wconf := WirelessNetworkConfig{ssid: SSID, psk: PSK}
	wconf.device.Type = WiFi

	return wconf
}

func NewWirelessConfigFromNetworkConfig(SSID string, PSK string, networkconf NetworkConfig) WirelessNetworkConfig {
	conf := NewWirelessNetworkConfig(SSID, PSK)
	conf.NetworkConfig = networkconf

	// Override the type from the provided networkConfig
	conf.device.Type = WiFi
	return conf
}

type GsmNetworkConfig struct {
	apn      string
	username string
	password string
	NetworkConfig
}

func NewGSMNetworkConfig(APN string, Username string, Password string) GsmNetworkConfig {
	conf := GsmNetworkConfig{apn: APN, username: Username, password: Password}
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

	// CreateConnection create a connection a connection
	CreateConnection(config interface{}) error

	// Private
	initialize() error
}

func NewService(sysdc *systemd.Connector) (NetworkService, error) {
	if sysdc == nil || !sysdc.Connected() {
		return nil, &dbuscon.NotConnectedError{}
	}

	// todo: First try dbus, if fails cli
	e := &networkDbusService{conn: sysdc.GetRawDbusConnection()}
	return e, e.initialize()
}
