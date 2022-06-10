package dbusclient

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

type dbusClient struct {
	conn    *dbus.Conn
	lastErr error
}

func (d *dbusClient) Close() {
	d.conn.Close()
}

func (d *dbusClient) Connect() {
	d.conn, d.lastErr = dbus.ConnectSystemBus()
	if d.lastErr != nil {
		fmt.Fprintln(os.Stderr, "Failed to connect to system bus:", d.lastErr)
		os.Exit(1)
	}
}

func (d *dbusClient) GetConnection() *dbus.Conn {
	return d.conn
}

func NewDbusClient() *dbusClient {
	return &dbusClient{}
}
