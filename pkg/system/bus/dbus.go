package bus

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

type DbusClient struct {
	conn    *dbus.Conn
	lastErr error
}

func (d *DbusClient) Shutdown() {
	if d.conn == nil {
		return
	}

	d.conn.Close()
}

func (d *DbusClient) Connect() {
	d.conn, d.lastErr = dbus.ConnectSystemBus()
	if d.lastErr != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to system bus:", d.lastErr)
		os.Exit(1)
	}
}

func (d *DbusClient) GetConnection() *dbus.Conn {
	return d.conn
}

func NewDbusClient() *DbusClient {
	return &DbusClient{}
}
