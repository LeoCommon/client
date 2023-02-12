package net

import (
	"fmt"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	gonm "github.com/Wifx/gonetworkmanager/v2"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

type networkDbusService struct {
	conn *dbus.Conn
	nm   gonm.NetworkManager
}

// Rudimentary WiFi connector
func (n *networkDbusService) ConnectToWiFi(targetSSID string, psk string) {
	devices, err := n.nm.GetPropertyAllDevices()
	if err != nil {
		return
	}

	for _, device := range devices {
		deviceType, err := device.GetPropertyDeviceType()
		if err != nil {
			return
		}

		// We found our wifi device
		if deviceType == gonm.NmDeviceTypeWifi {
			wdev, err := gonm.NewDeviceWireless(device.GetPath())
			if err != nil {
				return
			}

			wdev.RequestScan()
			// #todo this should go into some signal handler

			aps, err := wdev.GetAllAccessPoints()
			if err != nil {
				return
			}

			for _, ap := range aps {
				ssid, _ := ap.GetPropertySSID()

				if ssid == targetSSID {
					connection := make(map[string]map[string]interface{})
					connection["802-11-wireless"] = make(map[string]interface{})
					connection["802-11-wireless"]["security"] = "802-11-wireless-security"
					connection["802-11-wireless-security"] = make(map[string]interface{})
					connection["802-11-wireless-security"]["key-mgmt"] = "wpa-psk"
					connection["802-11-wireless-security"]["psk"] = psk
					connection["connection"] = make(map[string]interface{})
					connection["connection"]["id"] = "satos-probe" // #todo dynamic

					n.nm.AddAndActivateWirelessConnection(connection, device, ap)
				}
			}
		}
	}

}

// Obtain all active connections in the system
func (n *networkDbusService) getActiveConnections() []gonm.ActiveConnection {
	activeConnections, err := n.nm.GetPropertyActiveConnections()
	if err != nil {
		apglog.Error("Could not get active connections from NetworkManager", zap.Error(err))
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
	checkAvailable, err := s.nm.GetPropertyConnectivityCheckEnabled()
	if err != nil || !checkAvailable {
		apglog.Debug("NM does not have connectivity checking enabled", zap.Error(err))

		// Fall-back method
		return s.hasSingleActiveConnection()
	}

	nmConnectivity, err := s.nm.GetPropertyConnectivity()
	if err != nil {
		apglog.Error("failure during connectivity check", zap.Error(err))
		return false
	}

	// Check if we have full connectivity
	apglog.Info("connectivity check finished", zap.String("state", nmConnectivity.String()))
	return nmConnectivity == gonm.NmConnectivityFull
}

func (s *networkDbusService) Shutdown() {

}

func (s *networkDbusService) initialize() error {
	nm, err := gonm.NewNetworkManager()
	if err != nil {
		return err
	}

	s.nm = nm
	return nil
}
