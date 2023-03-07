package constants

import "time"

const (
	SYSTEMD_NOTIFY_SOCKET_ENV_VAR = "NOTIFY_SOCKET"
	SYSTEMD_NOTIFY_WATCHDOG       = "WATCHDOG=1"
	SYSTEMD_NOTIFY_RELOADING      = "RELOADING=1"
	SYSTEMD_NOTIFY_STOPPING       = "STOPPING=1"
	SYSTEMD_NOTIFY_READY          = "READY=1"

	SYSTEMD_SERVICE_STATE_ACTIVE      = "active"
	SYSTEMD_SERVICE_START_RESULT_DONE = "done"

	NETWORK_MANAGER_USER_CONFIG_DIRECTORY = "/etc/NetworkManager/system-connections/"
	NETWORK_MANAGER_ACTIVATION_TIMEOUT    = 60 * time.Second
)
