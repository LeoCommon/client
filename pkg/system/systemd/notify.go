package systemd

import (
	"errors"
	"net"
	"os"

	"disco.cs.uni-kl.de/apogee/pkg/constants"
	"disco.cs.uni-kl.de/apogee/pkg/log"
)

func EntertainWatchdog() error {
	log.Debug("Notifying systemd watchdog")
	return SystemdNotify(constants.SYSTEMD_NOTIFY_WATCHDOG)
}

func SystemdNotify(msg string) error {
	name := os.Getenv(constants.SYSTEMD_NOTIFY_SOCKET_ENV_VAR)
	if name == "" {
		return errors.New("systemd-notify socket was not available")
	}

	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Net: "unixgram", Name: name})
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(msg))
	return err
}
