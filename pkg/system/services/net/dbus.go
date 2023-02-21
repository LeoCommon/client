package net

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/constants"
	"disco.cs.uni-kl.de/apogee/pkg/system/misc"
	gonm "github.com/Wifx/gonetworkmanager/v2"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

type networkDbusService struct {
	conn     *dbus.Conn
	nm       gonm.NetworkManager
	settings gonm.Settings
}

// A simple priority based list
type ConfigItem struct {
	Prio   int
	Conn   gonm.Connection
	Device gonm.Device
}
type ConfigPrioList []ConfigItem

// This function searches for the highest priority network device and disconnects the rest afterwards
func (n *networkDbusService) EnforceNetworkPriority() error {
	workingDevice, err := n.FindWorkingConnection(nil)
	if err != nil {
		return err
	}

	devices, err := n.nm.GetAllDevices()
	if err != nil {
		return err
	}

	for _, dev := range devices {
		// Skip the working device
		if dev.GetPath() == workingDevice.GetPath() {
			continue
		}

		devType, err := dev.GetPropertyDeviceType()
		if err != nil {
			apglog.Error("failed to retrieve device type for device", zap.Error(err), zap.String("device", string(dev.GetPath())))
			continue
		}

		// Skip generic aka unknown devices (lo0)
		if devType == gonm.NmDeviceTypeGeneric {
			continue
		}

		activeConnection, err := dev.GetPropertyActiveConnection()
		if err == nil && activeConnection != nil {
			apglog.Info("disconnecting network device", zap.String("device", string(dev.GetPath())))
			if err := dev.Disconnect(); err != nil {
				apglog.Error("error while disconnecting device", zap.String("device", string(dev.GetPath())))
			}
		}
	}

	return nil
}

/*
Reloads network configurations from disk, this allows manual edits by the user to be reflected.
*/
func (n *networkDbusService) ReloadConfigurations() error {
	// Reload connections from within NetworkManager
	err := n.settings.ReloadConnections()
	if err != nil {
		apglog.Error("failed to reload connections", zap.Error(err))
		return err
	}

	return nil
}

/*
Network connectivity configurator, returns the device that was successfully enabled.
This only checks if the device has a valid IP Configuration, internet connectivity is not verified here.
This method should only be run, if no automatic connectivity (see autoconnect) entries are available in the configs.
*/
func (n *networkDbusService) FindWorkingConnection(netType *NetworkInterfaceType) (gonm.Device, error) {
	// Reload configurations
	n.ReloadConfigurations()

	// Retrieve all devices
	devices, err := n.nm.GetAllDevices()

	if err != nil {
		apglog.Error("system does not have a network device?!", zap.Error(err))
		return nil, err
	}

	// Hard Coded "importance"
	// 1) Wired 2) WiFi 3) Modem/GSM
	getPriority := func(deviceType gonm.NmDeviceType) int {
		switch deviceType {
		case gonm.NmDeviceTypeEthernet:
			return 10
		case gonm.NmDeviceTypeWifi:
			return 20
		case gonm.NmDeviceTypeModem:
			return 30
		}

		return -1
	}

	// Create priority based list
	configList := ConfigPrioList{}

	specificTypeRequested := netType != nil

	var singleType gonm.NmDeviceType
	if specificTypeRequested {
		singleType, err = mapDeviceTypeToNM(*netType)
		if err != nil {
			return nil, err
		}
	}

	// Loop through the devices
	for _, dev := range devices {
		// Get the device type
		devType, err := dev.GetPropertyDeviceType()
		if err != nil {
			apglog.Error("failed to retrieve device type for device", zap.Error(err), zap.String("device", string(dev.GetPath())))
			continue
		}

		// Skip generic aka unknown devices (lo0)
		if devType == gonm.NmDeviceTypeGeneric {
			continue
		}

		// Skip devices that were not requested
		if specificTypeRequested && singleType != devType {
			apglog.Info("skipping available device because the user requested it", zap.String("type", singleType.String()))
			continue
		}

		// Assign priority to network device
		prio := getPriority(devType)
		if prio < 0 {
			apglog.Warn("Skipping device due to unsupported type", zap.Int("type", int(devType)), zap.String("device", string(dev.GetPath())))
			continue
		}

		// Get all the available connections for this device
		connections, err := dev.GetPropertyAvailableConnections()
		if err != nil {
			apglog.Error("could not retrieve available connections", zap.Error(err), zap.String("device", string(dev.GetPath())))
			continue
		}

		// Loop through all connections to find a new one
		for _, con := range connections {
			dbpath := con.GetPath()
			filename, err := con.GetPropertyFilename()
			if err != nil {
				apglog.Warn("no file associated with connection, skipping", zap.Error(err), zap.String("connection", string(dbpath)))
				continue
			}

			// Safeguard: If the user assigned an autoconnect-priority to his user config, STOP this method.
			if netType == nil && strings.HasPrefix(filename, constants.NETWORK_MANAGER_USER_CONFIG_DIRECTORY) {
				settings, _ := con.GetSettings()
				if val, ok := settings["connection"]["autoconnect-priority"]; ok {
					apglog.Info("autoconnect-priority found, aborting manual search", zap.Int32("priority", val.(int32)), zap.String("file", filename))
					return nil, errors.New("autoconnect-priority found")
				}
			}

			apglog.Debug("found connection", zap.Int("priority", prio), zap.String("filename", filename), zap.String("connection", string(dbpath)))
			configList = append(configList, ConfigItem{Prio: prio, Conn: con, Device: dev})
		}
	}

	// Sort the list to respect the priorities
	sort.Slice(configList, func(i, j int) bool {
		return configList[i].Prio < configList[j].Prio
	})

	// Find a working configuration from the list
	for _, config := range configList {
		dev := config.Device
		con := config.Conn

		// Skip already active devices with the same connection
		// We do this here so we honor the priorities
		activeConnection, err := dev.GetPropertyActiveConnection()
		if err == nil && activeConnection != nil {
			connection, err := activeConnection.GetPropertyConnection()

			// Check if the connection paths are identical
			if err == nil && connection.GetPath() == con.GetPath() {
				apglog.Info("success skipping activation of already activated connection", zap.String("activeconnection", string(activeConnection.GetPath())), zap.String("device", string(dev.GetPath())))
				return dev, nil
			}
		}

		activeConnection, err = n.nm.ActivateConnection(con, dev, nil)
		if err != nil {
			apglog.Error("failed to activate connection", zap.Error(err), zap.String("connection", string(config.Conn.GetPath())), zap.String("device", string(dev.GetPath())))
			continue
		}

		// Check if the connection is properly activated
		activated, err := waitUntilConnectionIsActivated(activeConnection, constants.NETWORK_MANAGER_ACTIVATION_TIMEOUT)
		if activated {
			apglog.Info("connection activated after listening", zap.String("connection", string(con.GetPath())), zap.String("device", string(config.Device.GetPath())))
			return dev, nil
		}

		// Nothing more we can do
		apglog.Info("connection activation failed", zap.Error(err), zap.String("connection", string(con.GetPath())), zap.String("device", string(config.Device.GetPath())))
	}

	apglog.Warn("No connections could be configured")
	return nil, errors.New("no suitable connections found")
}

const (
	ethernetSection                      = "802-3-ethernet"
	wifiSection                          = "802-11-wireless"
	securitySection                      = "security"
	wifiSecuritySection                  = "802-11-wireless-security"
	gsmSection                           = "gsm"
	gsmSectionAPN                        = "apn"
	gsmSectionUsername                   = "username"
	gsmSectionPassword                   = "password"
	ethernetSectionAutoNegotiate         = "auto-negotiate"
	connectionSection                    = "connection"
	connectionSectionID                  = "id"
	connectionSectionType                = "type"
	connectionSectionUUID                = "uuid"
	connectionSectionIfaceName           = "interface-name"
	connectionSectionAutoconnect         = "autoconnect"
	connectionSectionAutoconnectPriority = "autoconnect-priority"
	connectionSectionAutoconnectRetries  = "autoconnect-retries"
	ip4Section                           = "ipv4"
	ip6Section                           = "ipv6"
	ip6SectionAddrGenMode                = "addr-gen-mode"
	ip6SectionAddrGenEUI64               = "0"
	ipSectionAddressData                 = "address-data"
	ipSectionAddress                     = "address"
	ipSectionAddresses                   = "addresses"
	ipSectionDhcpHostname                = "dhcp-hostname"
	ipSectionGateway                     = "gateway"
	ipSectionDns                         = "dns"
	ipSectionPrefix                      = "prefix"
	ipMethod                             = "method"
	ipMethodAuto                         = "auto"
	ipMethodManual                       = "manual"
	ipMethodDisabled                     = "disabled"
)

/*
	Converts ip4 to Numerical

Unchecked helper function, do not use if you dont know exactly what you are doing
*/
func ip4ToNumerical(ip4 netip.Addr) uint32 {
	// go-dbus sends in native system endianess
	return misc.NativeEndianess().Uint32(ip4.AsSlice())
}

// Converts an ipv4 string without prefix to its uint32 DBUS representation
func v4StrToNumerical(ip string) (uint32, error) {
	ip4, err := netip.ParseAddr(ip)
	if err != nil {
		return 0, errors.New("invalid ip passed")
	}

	return v4ToNumerical(ip4)
}

// Converts an ipv4 string without prefix to its uint32 DBUS representation
func v4ToNumerical(ip4 netip.Addr) (uint32, error) {
	if !ip4.Is4() {
		return 0, errors.New("could not convert to ipv4, did you mistake this for ipv6?")
	}

	return ip4ToNumerical(ip4), nil
}

// Converts an ipv6 string without prefix to its byte slice DBUS representation
func v6StrToByteSlice(ip string) ([]byte, error) {
	ip6, err := netip.ParseAddr(ip)
	if err != nil {
		return nil, errors.New("invalid ip passed")
	}

	return v6ToByteSlice(ip6)
}

func v6ToByteSlice(ip6 netip.Addr) ([]byte, error) {
	if !ip6.Is6() {
		return nil, errors.New("could not convert to ipv6, did you mistake this for ipv4?")
	}

	return ip6.AsSlice(), nil
}

// Checks if a connection is properly getting activated
func waitUntilConnectionIsActivated(activeConnection gonm.ActiveConnection, timeout time.Duration) (bool, error) {

	// #todo this once failed during testing, maybe we can just subscribe to changes in that case?
	state, err := activeConnection.GetPropertyState()
	if err != nil {
		return false, err
	}

	// Check if the connection is already activated
	if state == gonm.NmActiveConnectionStateActivated {
		apglog.Debug("connection activated", zap.String("connection", string(activeConnection.GetPath())))
		return true, nil
	}

	stateEvents := make(chan gonm.StateChange)
	exitEvent := make(chan struct{})

	// Quickly subscribe to the connection and check if it gets activated
	if activeConnection.SubscribeState(stateEvents, exitEvent) != nil {
		return false, errors.New("failed to subscribe to connection state changes")
	}

	// Wait until the timeout elapsed
	activationTimedOut := time.After(timeout)

	// Wait for device activation or timeout
	stateOkay := false
	err = nil
	func() {
		for {
			select {
			case <-activationTimedOut:
				// activation timed out
				err = errors.New("timeout while waiting for device activation")
				return
			case state, ok := <-stateEvents:
				if !ok {
					// channel was closed
					err = errors.New("channel got closed")
					return
				}
				apglog.Debug("received state change", zap.String("state", state.State.String()))

				// The connection was activated
				if state.State == gonm.NmActiveConnectionStateActivated {
					stateOkay = true
					return
				}
			}
		}
	}()

	// Close the channel
	exitEvent <- struct{}{}
	close(exitEvent)

	if stateOkay {
		apglog.Info("connection activated after listening", zap.String("connection", string(activeConnection.GetPath())))
		return true, nil
	}

	return false, err
}

type DNSResult struct {
	V4 []uint32
	V6 [][]byte
}

// This function returns v4 and v6 dns addresses in their network manager representations
func (n *networkDbusService) customDNSHandler(dns []string) (DNSResult, error) {
	if len(dns) == 0 {
		return DNSResult{}, nil
	}

	// Do not pre-allocate in case of invalid ips
	v4DNSs := make([]uint32, 0, len(dns))
	v6DNSs := make([][]byte, 0, len(dns))

	for _, dns := range dns {
		ip, err := netip.ParseAddr(dns)
		if err != nil {
			return DNSResult{}, err
		}

		// Determine if the ip is v4 or v6
		if ip.Is4() {
			v4DNSs = append(v4DNSs, ip4ToNumerical(ip))
		} else if ip.Is6() {
			v6DNSs = append(v6DNSs, ip.AsSlice())
		}
	}

	return DNSResult{V4: v4DNSs, V6: v6DNSs}, nil
}

/*
Create a static / dynamic v4 and v6 config inside the specified connection
This function does not perform input validation, please make sure you pass valid data to it
*/
func (n *networkDbusService) NMV4V6Config(connection map[string]map[string]interface{}, config NetworkConfig) error {
	connection[ip4Section] = make(map[string]interface{})

	// Parse ipv4 and ipv6 dns
	dnsResult, err := n.customDNSHandler(config.dnsServers)
	if err != nil {
		return err
	}

	if config.v4 != nil {
		if config.v4.Static != nil {
			ipv4 := config.v4.Static

			// commmonly used fields
			v4ip := ipv4.Network.Addr()
			v4prefixLen := uint32(ipv4.Network.Bits())

			addressData := make([]map[string]interface{}, 1)
			addressData[0] = make(map[string]interface{})
			addressData[0][ipSectionAddress] = v4ip.String()
			addressData[0][ipSectionPrefix] = v4prefixLen

			numericalIP, err := v4ToNumerical(v4ip)
			if err != nil {
				return err
			}

			desiredGatewayIP, err := v4ToNumerical(ipv4.Gateway)
			if err != nil {
				return err
			}

			// This order is specified by NetworkManager, we need to respect it
			addresses := make([]uint32, 3)
			addresses[0] = numericalIP
			addresses[1] = v4prefixLen
			addresses[2] = desiredGatewayIP

			addressArray := make([][]uint32, 1)
			addressArray[0] = addresses
			connection[ip4Section][ipSectionAddresses] = addressArray

			connection[ip4Section][ipSectionAddressData] = addressData
			connection[ip4Section][ipSectionGateway] = ipv4.Gateway.String()
			connection[ip4Section][ipMethod] = ipMethodManual
		} else {
			connection[ip4Section][ipMethod] = ipMethodAuto
		}

		// Add dns v4 servers
		if dnsResult.V4 != nil {
			connection[ip4Section][ipSectionDns] = dnsResult.V4
		}
	} else {
		connection[ip4Section][ipMethod] = ipMethodDisabled
	}

	connection[ip6Section] = make(map[string]interface{})
	if config.v6 != nil {
		if config.v6.Static != nil {
			ipv6 := config.v6.Static

			// commmonly used fields
			v6ip := ipv6.Network.Addr()
			v6prefixLen := uint32(ipv6.Network.Bits())

			addressData := make([]map[string]interface{}, 1)
			addressData[0] = make(map[string]interface{})
			addressData[0][ipSectionAddress] = v6ip.String()
			addressData[0][ipSectionPrefix] = v6prefixLen
			connection[ip6Section][ipSectionAddressData] = addressData

			byteSliceIP, err := v6ToByteSlice(v6ip)
			if err != nil {
				return err
			}

			byteSliceGateway, err := v6ToByteSlice(ipv6.Gateway)
			if err != nil {
				return err
			}

			// This order is specified by NetworkManager, we need to respect it
			addresses := make([]interface{}, 3)
			addresses[0] = byteSliceIP
			addresses[1] = v6prefixLen
			addresses[2] = byteSliceGateway

			addressArray := make([][]interface{}, 1)
			addressArray[0] = addresses
			connection[ip6Section][ipSectionAddresses] = addressArray
			connection[ip6Section][ipSectionGateway] = ipv6.Gateway.String()
			connection[ip6Section][ipMethod] = ipMethodManual
		} else {
			connection[ip6Section][ipMethod] = ipMethodAuto

			// Force EUI64 mode if the user specified it
			if config.v6.eui64 {
				connection[ip6Section][ip6SectionAddrGenMode] = ip6SectionAddrGenEUI64
			}
		}

		// Add dns v6 servers
		if dnsResult.V6 != nil {
			connection[ip6Section][ipSectionDns] = dnsResult.V6
		}
	} else {
		connection[ip6Section][ipMethod] = ipMethodDisabled
	}

	return nil
}

// This maps our type representation to the network manager internal one
func mapDeviceTypeToNM(interfaceType NetworkInterfaceType) (gonm.NmDeviceType, error) {
	switch interfaceType {
	case Ethernet:
		return gonm.NmDeviceTypeEthernet, nil
	case WiFi:
		return gonm.NmDeviceTypeWifi, nil
	case GSM:
		return gonm.NmDeviceTypeModem, nil
	}

	return gonm.NmDeviceTypeUnknown, fmt.Errorf("could not map type %s", interfaceType)
}

func (n *networkDbusService) GetExistingConnection(connectionUUID string) (*gonm.Connection, error) {
	settings, err := gonm.NewSettings()
	if err != nil {
		return nil, err
	}

	conns, err := settings.ListConnections()
	if err != nil {
		return nil, err
	}

	for _, v := range conns {
		connectionSettings, err := v.GetSettings()
		if err != nil {
			apglog.Error("error when accessing connection settings, continuing search", zap.Error(err))
			continue
		}
		cSection := connectionSettings[connectionSection]
		if cSection[connectionSectionUUID] == connectionUUID {
			return &v, nil
		}
	}

	return nil, nil
}

func (n *networkDbusService) activateConnection(settings map[string]map[string]interface{}, dev gonm.Device, ap *gonm.AccessPoint, existingConnection *gonm.Connection) (gonm.ActiveConnection, error) {
	// Activate the connection
	if existingConnection != nil {
		con := *existingConnection
		if err := con.Save(); err != nil {
			return nil, fmt.Errorf("could not save connection, aborting %v", err)
		}

		return n.nm.ActivateConnection(con, dev, nil)
	}

	// If this is a new connection and we get an AP its wireless
	if ap != nil {
		return n.nm.AddAndActivateWirelessConnection(settings, dev, *ap)
	}

	// The normal flow without AP
	return n.nm.AddAndActivateConnection(settings, dev)
}

// This function creates a connection based on the supplied network config
func (n *networkDbusService) CreateConnection(config interface{}) error {
	wifiConfig, isWifi := config.(wirelessNetworkConfig)
	gsmConfig, isGSM := config.(gsmNetworkConfig)
	wiredConfig, isWired := config.(NetworkConfig)

	if !isWifi && !isWired && !isGSM {
		return fmt.Errorf("invalid parameter for config")
	}

	// Get the generic network config
	var ipConf NetworkConfig
	if isWifi {
		ipConf = wifiConfig.NetworkConfig
	} else if isGSM {
		ipConf = gsmConfig.NetworkConfig
	} else if isWired {
		ipConf = wiredConfig
	}

	// Prevent invalid ip configuration
	if ipConf.v4 == nil && ipConf.v6 == nil {
		return fmt.Errorf("no v4 and/or v6 configuration specified, aborting")
	}

	connectionTypeStr := string(ipConf.device.Type)
	internalNMDeviceType, err := mapDeviceTypeToNM(ipConf.device.Type)
	if err != nil {
		apglog.Error("error during device type mapping", zap.Error(err))
		return err
	}

	// Retrieve all devices from the system
	nmDevices, err := n.nm.GetAllDevices()
	if err != nil {
		return err
	}

	// Try to find a matching device, either by name or by type
	targetDeviceName := ipConf.device.Name
	deviceNameIsEmpty := len(targetDeviceName) == 0
	var nmDevice gonm.Device = nil
	for _, dev := range nmDevices {
		intf, err := dev.GetPropertyInterface()
		if err != nil {
			apglog.Warn("could not get interface for device", zap.Error(err), zap.String("device", string(dev.GetPath())))
			continue
		}

		devtype, err := dev.GetPropertyDeviceType()
		if err != nil {
			apglog.Warn("skipping device, could not get type", zap.String("device", string(dev.GetPath())))
			continue
		}

		// If no interface name was specified only try to find one with the right type
		var deviceNameMatches bool = true
		if !deviceNameIsEmpty {
			deviceNameMatches = intf == targetDeviceName
		}

		// Device with matching name and device type found
		if deviceNameMatches && devtype == internalNMDeviceType {
			nmDevice = dev
			// (Re-)populate target device name because we matched
			targetDeviceName = intf
			break
		}
	}

	// If we found no suitable device, we have to return
	if nmDevice == nil {
		apglog.Error("could not find device for network configuration", zap.String("type", connectionTypeStr), zap.String("name", targetDeviceName))
		return errors.New("no valid device found for configuration")
	}

	apglog.Info("device found for new network configuration", zap.String("device", string(nmDevice.GetPath())), zap.String("type", connectionTypeStr), zap.String("name", targetDeviceName))

	// Create skeleton
	var existingConnection *gonm.Connection = nil

	// This also overwrites old settings if we modify an existing config
	settings := make(map[string]map[string]interface{})
	settings[connectionSection] = make(map[string]interface{})
	settings[connectionTypeStr] = make(map[string]interface{})
	csec := settings[connectionSection]

	uuid := ipConf.settings.UUID
	// Determine if we already have a connection based on this UUID
	if uuid != nil {
		uuidStr := uuid.String()
		apglog.Debug("uuid specified, searching for match", zap.String("uuid", uuidStr))

		// #todo also allow matching by ID aka name here
		// else this could use n.settings.GetConnectionByUUID()
		con, err := n.GetExistingConnection(uuidStr)
		if err != nil {
			apglog.Error("search for existing connection failed", zap.Error(err))
		}

		// Save the existing connection
		if con != nil {
			apglog.Info("re-using existing connection with uuid", zap.String("uuid", uuidStr))
			existingConnection = con
		}

		// Save the UUID
		csec[connectionSectionUUID] = uuidStr
	}

	// Set connectionID, typeStr
	csec[connectionSectionID] = ipConf.settings.Name
	csec[connectionSectionType] = connectionTypeStr

	// Set target interface name if it was provided
	if !deviceNameIsEmpty {
		csec[connectionSectionIfaceName] = targetDeviceName
	}

	if autoConnect := ipConf.settings.AutoConnect; autoConnect != nil {
		csec[connectionSectionAutoconnect] = autoConnect.State

		// Assign auto connect priority if provided
		if prio := autoConnect.Priority; prio != nil {
			csec[connectionSectionAutoconnectPriority] = autoConnect.Priority
		}

		// Assign auto connect priority if provided
		if retries := autoConnect.Retries; retries != nil {
			csec[connectionSectionAutoconnectRetries] = autoConnect.Retries
		}
	}

	// Populate the ipv4 and ipv6 config sections
	err = n.NMV4V6Config(settings, ipConf)
	if err != nil {
		return err
	}

	// Set the required device properties for wifi, gsm, wired
	deviceSection := settings[connectionTypeStr]

	var wifiAP *gonm.AccessPoint = nil
	var activeConnection gonm.ActiveConnection = nil
	var activateError error

	if isWifi {
		// Wireless specific settings
		deviceSection[securitySection] = wifiSecuritySection
		settings[wifiSecuritySection] = make(map[string]interface{})

		// For now only wpa-psk is supported
		settings[wifiSecuritySection]["key-mgmt"] = "wpa-psk"
		settings[wifiSecuritySection]["psk"] = wifiConfig.psk

		wdev, err := gonm.NewDeviceWireless(nmDevice.GetPath())
		if err != nil {
			return errors.New("failed to get access to wireless device")
		}

		// Ignore scan errors
		wdev.RequestScan()

		// Grab all access points in order to find the SSID
		aps, err := wdev.GetAllAccessPoints()
		if err != nil {
			return errors.New("could not get access points for wifi configuration")
		}

		for _, ap := range aps {
			ssid, _ := ap.GetPropertySSID()

			if ssid == wifiConfig.ssid {
				wifiAP = &ap
				break
			}
		}

		if wifiAP == nil {
			return fmt.Errorf("could not find target SSID \"%s\"", wifiConfig.ssid)
		}
	} else if isGSM {
		deviceSection[gsmSectionAPN] = gsmConfig.APN
		deviceSection[gsmSectionUsername] = gsmConfig.Username
		deviceSection[gsmSectionPassword] = gsmConfig.Password
	}

	// Activate the connection, this creates the file on disk
	activeConnection, activateError = n.activateConnection(settings, nmDevice, wifiAP, existingConnection)

	// Check for errors during activation
	if activateError != nil {
		apglog.Error("connection could not be activated", zap.Error(activateError))
		return err
	}

	oCon, err := activeConnection.GetPropertyConnection()
	if err != nil || oCon == nil {
		apglog.Error("could not get connection property from active connection")
		return err
	}

	apglog.Info("connection set-up, waiting for activation", zap.String("connection", string(activeConnection.GetPath())))
	apglog.Debug("dumping connection info", zap.Any("settings", settings))

	// Check if the connection was properly activated
	activated, err := waitUntilConnectionIsActivated(activeConnection, constants.NETWORK_MANAGER_ACTIVATION_TIMEOUT)

	// If the config was not properly activated, delete it
	if !activated {
		if oCon != nil {
			apglog.Info("deleting invalid connection")
			err = oCon.Delete()
		} else {
			apglog.Error("could not delete failed connection")
		}
	}

	// #todo perform additional checks on activeConnection

	// Flush to disk, this should be the last thing we do
	n.ReloadConfigurations()

	return err
}

// Obtain all active connections in the system
func (n *networkDbusService) getActiveConnections() []gonm.ActiveConnection {
	activeConnections, err := n.nm.GetPropertyActiveConnections()
	if err != nil {
		apglog.Error("could not get active connections from NetworkManager", zap.Error(err))
		return nil
	}

	return activeConnections
}

func (n *networkDbusService) getActiveConnectionByType(t NetworkInterfaceType) gonm.ActiveConnection {
	for _, con := range n.getActiveConnections() {
		conT, err := con.GetPropertyType()
		if err != nil {
			apglog.Warn("Skipping active network connections due to error", zap.Error(err))
			continue
		}

		if conT == string(t) {
			return con
		}
	}

	return nil
}

func (n *networkDbusService) getAvailableConnections() []gonm.Connection {
	availableConnections, err := n.settings.ListConnections()
	if err != nil {
		apglog.Error("Could not get list of connections from NetworkManager", zap.Error(err))
		return nil
	}

	return availableConnections
}

// Finds all available connections for a specific network type
func (n *networkDbusService) getAvailableConnectionsByType(t NetworkInterfaceType) []gonm.Connection {
	cons := make([]gonm.Connection, 0)
	for _, con := range n.getAvailableConnections() {
		settings, err := con.GetSettings()
		if err != nil {
			apglog.Warn("could not get connection settings, skipping", zap.Error(err))
			continue
		}

		cs, ok := settings[connectionSection]
		if !ok {
			apglog.Warn("connection did not have connection section")
			continue
		}

		ct, ok := cs[connectionSectionType]
		if !ok {
			apglog.Warn("connection did not have a type")
			continue
		}

		// If the connection type equals the type, we found a match
		if ct == t {
			cons = append(cons, con)
		}
	}

	return cons
}

type ConnectionNotAvailable struct {
	connectionType NetworkInterfaceType // optional
}

func (e *ConnectionNotAvailable) Error() string {
	return fmt.Sprintf("connection with type %v: not available", string(e.connectionType))
}

// Gets the connection state for a specified type
func (n *networkDbusService) GetConnectionStateByType(netifType NetworkInterfaceType) (gonm.NmActiveConnectionState, error) {
	ac := n.getActiveConnectionByType(netifType)
	if ac == nil {
		return gonm.NmActiveConnectionStateUnknown, &ConnectionNotAvailable{netifType}
	}

	return n.GetConnectionState(ac)
}

// Returns the connection State of an active connection
func (n *networkDbusService) GetConnectionState(conn gonm.ActiveConnection) (gonm.NmActiveConnectionState, error) {
	if conn == nil {
		return gonm.NmActiveConnectionStateUnknown, &ConnectionNotAvailable{}
	}

	conState, err := conn.GetPropertyState()
	if conn == nil {
		return gonm.NmActiveConnectionStateUnknown, err
	}

	return conState, nil
}

func activeConnectionStateToString(r gonm.NmActiveConnectionState) string {
	switch r {
	case gonm.NmActiveConnectionStateUnknown:
		return "unknown"
	case gonm.NmActiveConnectionStateActivating:
		return "preparing"
	case gonm.NmActiveConnectionStateActivated:
		return "active"
	case gonm.NmActiveConnectionStateDeactivating:
		return "deactivating"
	case gonm.NmActiveConnectionStateDeactivated:
		return "deactivated"
	}

	// Fallback
	return "unknown_nm_broken"
}

// Returns the connection state of the selected type as String
func (s *networkDbusService) GetConnectionStateStr(netifType NetworkInterfaceType) (string, error) {
	r, err := s.GetConnectionStateByType(netifType)
	if err != nil {
		return "not_configured", err
	}

	return activeConnectionStateToString(r), nil
}

// Checks if the supplied network type is active
func (n *networkDbusService) IsNetworkTypeActive(netifType NetworkInterfaceType) (state bool, err error) {
	s, err := n.GetConnectionStateByType(netifType)
	if err != nil {
		return false, err
	}

	return s == gonm.NmActiveConnectionStateActivated, nil
}

// Checks if the system has at-least one functioning active connection
func (n *networkDbusService) hasSingleActiveConnection() (state bool) {
	for _, con := range n.getActiveConnections() {
		state, err := n.GetConnectionState(con)

		// Bail if we found one active connection
		if err == nil && state == gonm.NmActiveConnectionStateActivated {
			return true
		}
	}

	return false
}

// Checks if the overall system has connectivity
func (s *networkDbusService) HasConnectivity() (state bool) {
	// Leverage the connectivity check if available
	checkAvailable, err := s.nm.GetPropertyConnectivityCheckEnabled()
	if err != nil || !checkAvailable {
		apglog.Debug("NM does not have connectivity checking enabled", zap.Error(err))

		// Fall-back to checking if there is a single active connection
		return s.hasSingleActiveConnection()
	}

	nmConnectivity, err := s.nm.GetPropertyConnectivity()
	if err != nil {
		apglog.Error("failure during connectivity check", zap.Error(err))
		return false
	}

	// Check if we have full connectivity
	apglog.Debug("connectivity check finished", zap.String("state", nmConnectivity.String()))
	return nmConnectivity == gonm.NmConnectivityFull
}

func (n *networkDbusService) SetDeviceStateByType(devtype NetworkInterfaceType, enable bool) error {
	// If the user wants to disable a connection, we can disable the currently active one
	devices, err := n.nm.GetPropertyAllDevices()
	if err != nil {
		return err
	}

	// Map our simplified device type to the NM representation
	mappedType, err := mapDeviceTypeToNM(devtype)
	if err != nil {
		return err
	}

	var device gonm.Device = nil
	for _, dev := range devices {
		// Retrieve device type
		deviceType, err := dev.GetPropertyDeviceType()
		if err != nil {
			return err
		}

		if deviceType == mappedType {
			device = dev
			break
		}
	}

	// If we just need to disconnect
	if !enable {
		return device.Disconnect()
	}

	// If the user wants to connect the device we have to find a working connection first
	_, err = n.FindWorkingConnection(&devtype)
	return err
}

func (s *networkDbusService) Shutdown() {
}

func (s *networkDbusService) initialize() error {
	nm, err := gonm.NewNetworkManager()
	if err != nil {
		return err
	}

	s.nm = nm

	settings, err := gonm.NewSettings()
	if err != nil {
		apglog.Error("could not connect to network manager settings, aborting", zap.Error(err))
		return err
	}

	s.settings = settings
	return nil
}
