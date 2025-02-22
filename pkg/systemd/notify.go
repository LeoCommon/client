package systemd

import (
	"errors"
	"net"
	"os"

	"github.com/LeoCommon/client/pkg/log"
)

// EntertainWatchdog sends a notification to the systemd watchdog
func EntertainWatchdog() error {
	log.Debug("Notifying systemd watchdog")
	return Notify(NotifyWatchdog)
}

// Notify sends the provided msg to the systemd socket
func Notify(msg string) error {
	name := os.Getenv(NotifySocketEnvVar)
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
