package dbuscon

import (
	"fmt"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"go.uber.org/zap"

	"github.com/godbus/dbus/v5"
)

type NotConnectedError struct{}

func (e *NotConnectedError) Error() string {
	return "client is not connected"
}

func (e *NotConnectedError) Is(target error) bool {
	_, ok := target.(*NotConnectedError)
	return ok
}

type Client struct {
	conn    *dbus.Conn
	lastErr error
}

func (d *Client) Shutdown() (err error) {
	if d.conn == nil {
		return
	}

	err = d.conn.Close()
	d.conn = nil
	return
}

// Reconnect re-establishes the system connection if its down
func (d *Client) Reconnect() error {
	if d.conn != nil && d.conn.Connected() {
		return fmt.Errorf("connection is active and working, not reconnecting")
	}

	// Connection seems to be down, close it again
	_ = d.conn.Close()

	// Re-connect
	return d.Connect()
}

// Connected returns whether the bus connection is established or not
func (d *Client) Connected() (*dbus.Conn, bool) {
	return d.conn, d.conn != nil && d.conn.Connected()
}

func (d *Client) Connect() error {
	d.conn, d.lastErr = dbus.ConnectSystemBus()
	if d.lastErr != nil {
		log.Error("Failed to connect to system bus", zap.Error(d.lastErr))
	}

	return d.lastErr
}

func (d *Client) GetConnection() *dbus.Conn {
	return d.conn
}

func NewDbusClient() *Client {
	return &Client{}
}
