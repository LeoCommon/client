package systemd

import (
	"time"
)

const (
	NotifySocketEnvVar = "NOTIFY_SOCKET"
	NotifyWatchdog     = "WATCHDOG=1"
	NotifyReloading    = "RELOADING=1"
	NotifyStopping     = "STOPPING=1"
	NotifyReady        = "READY=1"

	ServiceStateActive = "active"
	JobResultDone      = "done"

	BusObjectPropertyActiveState = "ActiveState"

	BusObjectSystemdDest     = "org.freedesktop.systemd1"
	BusObjectSystemdDestUnit = BusObjectSystemdDest + ".Unit"
	BusObjectSystemdPath     = "/org/freedesktop/systemd1"
	BusObjectSystemdUnitTmpl = "/org/freedesktop/systemd1/unit/%s"

	BusMemberGetProp = "org.freedesktop.DBus.Properties.Get"

	// Systemd Manager actions
	BusManagerInterface     = BusObjectSystemdDest + ".Manager"
	BusInterfaceStartUnit   = BusManagerInterface + ".StartUnit"
	BusInterfaceStopUnit    = BusManagerInterface + ".StopUnit"
	BusInterfaceRestartUnit = BusManagerInterface + ".RestartUnit"

	// Signals
	BusMemberJobRemoved      = "JobRemoved"
	BusSignalJobRemoved      = BusManagerInterface + "." + BusMemberJobRemoved
	BusMemberJobNew          = "JobNew"
	BusMemberUnitNew         = "UnitNew"
	BusMemberUnitRemoved     = "UnitRemoved"
	BusMemberStartupFinished = "StartupFinished"

	NetworkManagerUserConfigDirectory = "/etc/NetworkManager/system-connections/"
	NetworkManagerActivationTimeout   = 60 * time.Second
)
