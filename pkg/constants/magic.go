package constants

import "time"

const (
	// Apogee magic values
	APOGEE_SERVICE_NAME = "apogee-client.service"

	SYSTEMD_NOTIFY_SOCKET_ENV_VAR = "NOTIFY_SOCKET"
	SYSTEMD_NOTIFY_WATCHDOG       = "WATCHDOG=1"
	SYSTEMD_NOTIFY_RELOADING      = "RELOADING=1"
	SYSTEMD_NOTIFY_STOPPING       = "STOPPING=1"
	SYSTEMD_NOTIFY_READY          = "READY=1"

	REBOOT_PENDING_TMPFILE = "/tmp/.reboot-pending"
	DONT_RESTART_EXIT_CODE = 0

	NETWORK_MANAGER_USER_CONFIG_DIRECTORY = "/etc/NetworkManager/system-connections/"
	NETWORK_MANAGER_ACTIVATION_TIMEOUT    = 60 * time.Second
)
