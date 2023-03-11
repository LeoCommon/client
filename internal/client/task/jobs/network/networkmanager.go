package network

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/misc"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"
	"go.uber.org/zap"
)

// todo: Lets please redo this entire API and do it in properly typed JSON
const (
	autoconnect = "autoconnect"
	methodv4    = "methodIPv4"
	v4manual    = "manual"
	v4auto      = "auto"
	v4disabled  = "disabled"
	addressesv4 = "addressesIPv4"
	dnsv4       = "dnsIPv4"
	gatewayv4   = "gatewayIPv4"
	// Wifi related configuration parameters
	wifissid = "ssid"
	wifipsk  = "psk"
	// No GSM support

	// Static UUIDs to re-use connections
	WiFiUUID     = "d5b77cad-412e-4998-a050-2dc37ebae382"
	EthernetUUID = "224e7b1b-5c9b-4106-9dcb-9e91e0bbe6f1"
	GSMUUID      = "e7ee2393-bf12-44ff-b8ac-b36324c9bc9b"

	// Keys for set_network_conn
	coneth  = "eth"
	conwifi = "wifi"
	congsm  = "gsm"
)

type NetworkJobIPData struct {
	Network     netip.Prefix
	Gateway     netip.Addr
	DNS         netip.Addr
	IPv4Method  string
	Autoconnect bool
}

// NewNetworkJobIPData Creates a new instance of the NetworkJobData with the defaults set
func NewNetworkJobIPData() *NetworkJobIPData {
	nJD := NetworkJobIPData{}
	nJD.Autoconnect = true
	nJD.IPv4Method = v4auto
	return &nJD
}

// ParseIPFromMap Parses an ip from the job map returns an error if the ip is invalid
func ParseIPFromMap(m map[string]string, idx string) (ip netip.Addr, err error) {
	// Start with empty default value
	ip = netip.Addr{}

	// Lookup IP and try to parse it
	if _ip, found := m[idx]; found {
		ip, err = netip.ParseAddr(_ip)
		if err != nil {
			log.Error("could not parse ip, invalid value", zap.String("ip", _ip))
		}
	}

	return
}

func ParseNetworkJobIPConfig(m map[string]string) (config *NetworkJobIPData, err error) {
	// assign default config
	config = NewNetworkJobIPData()

	// Check for autoconnect presence
	if ac, found := m[autoconnect]; found {
		config.Autoconnect, err = strconv.ParseBool(ac)
		if err != nil {
			log.Error("invalid value for autoconnect", zap.String("autoconnect", ac))
			return
		}
	} else {
		log.Info("autoconnect not specified, using defaults", zap.Bool("autoconnect", config.Autoconnect))
	}

	// Check for v4 method
	if v4method, found := m[methodv4]; found {
		switch v4method {
		case v4disabled:
			fallthrough
		case v4auto:
			fallthrough
		case v4manual:
			config.IPv4Method = v4method
		default:
			log.Error("invalid value for ipv4 method", zap.String("method", v4method))
			return nil, fmt.Errorf("invalid value for ipv4 method")
		}
	} else {
		log.Info("No method found, using defaults", zap.String("method", config.IPv4Method))
	}

	// If manual was specified we need to fill in the required settings
	if config.IPv4Method == v4manual {
		// Get Desired Address
		if ac, found := m[addressesv4]; found {
			config.Network, err = netip.ParsePrefix(ac)
			if err != nil {
				log.Error("invalid value for autoconnect", zap.Error(err))
				return
			}
		} else {
			err = fmt.Errorf("no address specified for manual configuration")
			return
		}

		// Get Desired Gateway
		config.Gateway, err = ParseIPFromMap(m, gatewayv4)
		if err != nil {
			log.Error("invalid value for gateway", zap.Error(err))
			return
		}
	}

	// Get Desired DNS, this field must exist
	config.DNS, err = ParseIPFromMap(m, dnsv4)
	if err != nil {
		log.Error("invalid value for dns", zap.Error(err))
		return
	}

	return
}

// SetConfig try to set and activate the new network configuration
func SetConfig(job api.FixedJob, app *client.App, netType net.NetworkInterfaceType) error {
	if app.Config.Jobs.Network.Disabled {
		return &jobs.DisabledError{}
	}

	m := job.Arguments
	data, err := ParseNetworkJobIPConfig(m)
	if err != nil {
		return err
	}

	// Set up a generic network configuration for both devices
	genericConfig := net.NewNetworkConfig()
	genericConfig.WithAutoconnect(&net.AutoConnectSettings{State: data.Autoconnect})

	switch data.IPv4Method {
	case v4disabled:
		log.Warn("disabling ipv4 on new connection", zap.Any("config", data))
	case v4auto:
		genericConfig.WithV4Automatic()
	case v4manual:
		genericConfig.WithV4Static(&net.V4Config{
			Static: &net.Static{
				Network: data.Network,
				Gateway: data.Gateway,
			},
		})
	}

	// If a valid dns was specified, use it
	if data.DNS.IsValid() {
		genericConfig.WithCustomDNS([]string{data.DNS.String()})
	}

	switch netType {
	case net.WiFi:
		var ssid string
		var psk string

		var found bool
		if ssid, found = m[wifissid]; !found {
			return fmt.Errorf("ssid not specified, aborting")
		}

		if psk, found = m[wifipsk]; !found {
			return fmt.Errorf("psk not specified, aborting")
		}

		// Create wireless configuration
		wConf := net.NewWirelessConfigFromNetworkConfig(ssid, psk, genericConfig)
		wConf.WithName("wifi_provisioned").WithUUID(WiFiUUID)
		return app.NetworkService.CreateConnection(wConf)
	case net.Ethernet:
		conf := net.NewWiredNetworkConfig()
		conf.WithName("eth_provisioned").WithUUID(EthernetUUID)
		return app.NetworkService.CreateConnection(conf)
	}

	return fmt.Errorf("invalid network type encountered %v", netType)
}

func SetNetworkConnectivity(job api.FixedJob, app *client.App) (err error) {
	if app.Config.Jobs.Network.Disabled {
		return &jobs.DisabledError{}
	}

	// Do not touch anything by default
	var ethState *bool = nil
	var wifiState *bool = nil
	var gsmState *bool = nil

	// iterate over key value pairs
	for k, v := range job.Arguments {
		//
		switch strings.ToLower(k) {
		case coneth:
			ethState, err = misc.ParseOnOffState(v)
			if err != nil {
				return err
			}
		case conwifi:
			wifiState, err = misc.ParseOnOffState(v)
			if err != nil {
				return err
			}
		case congsm:
			gsmState, err = misc.ParseOnOffState(v)
			if err != nil {
				return err
			}
		}
	}

	// fixme: this behavior is counter intuitive, we fail on the first error
	// I think it would be smarter to collect all errors and return them at the end
	if ethState != nil {
		err = app.NetworkService.SetDeviceStateByType(net.Ethernet, *ethState)
		if err != nil {
			return err
		}
	}

	if wifiState != nil {
		err := app.NetworkService.SetDeviceStateByType(net.WiFi, *wifiState)
		if err != nil {
			return err
		}
	}

	if gsmState != nil {
		err := app.NetworkService.SetDeviceStateByType(net.GSM, *gsmState)
		if err != nil {
			return err
		}
	}

	return err
}
