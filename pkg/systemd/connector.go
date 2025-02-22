package systemd

import (
	"context"
	"fmt"
	"sync"

	"github.com/LeoCommon/client/pkg/log"
	"github.com/LeoCommon/client/pkg/systemd/dbuscon"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

type Connector struct {
	sync.Mutex

	client            *dbuscon.Client
	signalCh          chan *dbus.Signal
	jobRemoveListener struct {
		sync.Mutex
		jobs    map[dbus.ObjectPath]chan<- string
		matcher []dbus.MatchOption
	}
}

// init initializes the systemd connector
func (c *Connector) init() error {
	c.jobRemoveListener.jobs = make(map[dbus.ObjectPath]chan<- string)
	c.client = dbuscon.NewDbusClient()

	c.jobRemoveListener.matcher = []dbus.MatchOption{
		dbus.WithMatchInterface(BusManagerInterface),
		dbus.WithMatchMember(BusMemberJobRemoved),
	}

	// Connect to dbus
	if err := c.client.Connect(); err != nil {
		return err
	}

	// Match all jobs removed signal so we can get their results
	conn := c.client.GetConnection()
	if conn == nil {
		log.Error("connection was nil")
		return &dbuscon.NotConnectedError{}
	}

	// Create a slightly buffered channel
	c.signalCh = make(chan *dbus.Signal, 10)
	conn.Signal(c.signalCh)

	// Start the signal listener
	go c.listenForSignals()

	// Add the match signal
	conn.AddMatchSignal(c.jobRemoveListener.matcher...)
	log.Info("systemd/dbus initialization complete")

	return nil
}

// Create a new connector for systemd
func NewConnector() (*Connector, error) {
	c := Connector{}
	c.Lock()
	defer c.Unlock()

	return &c, c.init()
}

// GetRawDbusClient returns the underlying dbus client of this instance, use wisely!
func (c *Connector) GetRawDbusClient() *dbuscon.Client {
	return c.client
}

// GetRawDbusConnection returns the currently active dbus connection
func (c *Connector) GetRawDbusConnection() *dbus.Conn {
	if !c.Connected() {
		return nil
	}

	return c.client.GetConnection()
}

func (c *Connector) Shutdown() error {
	err := c.client.GetConnection().RemoveMatchSignal(c.jobRemoveListener.matcher...)
	if err != nil {
		return err
	}

	// Close the signal channel
	close(c.signalCh)
	return nil
}

func (c *Connector) jobCompleteSignal(signal *dbus.Signal) {
	var id uint32
	var job dbus.ObjectPath
	var unit string
	var result string
	dbus.Store(signal.Body, &id, &job, &unit, &result)

	c.jobRemoveListener.Lock()
	// If a listener for this exists, inform it
	out, ok := c.jobRemoveListener.jobs[job]
	if ok {
		out <- result
		delete(c.jobRemoveListener.jobs, job)
	}
	c.jobRemoveListener.Unlock()
}

// manageUnits provides the base for common systemd unit management tasks
func (c *Connector) manageUnits(unitName string, method string, ctx context.Context, ch chan<- string) error {
	if !c.Connected() {
		return &dbuscon.NotConnectedError{}
	}

	// Grab the connection
	conn := c.client.GetConnection()

	// We do have the connection, lets try to restart the service now
	service := conn.Object(BusObjectSystemdDest, BusObjectSystemdPath)
	result := service.CallWithContext(ctx, method, 0, unitName, "replace")

	// Bail out early if we encountered an error
	if result.Err != nil {
		return result.Err
	}

	// If the user was interested in the result we need to register the callback
	if ch != nil {
		var p dbus.ObjectPath
		if err := result.Store(&p); err != nil {
			return err
		}
		c.jobRemoveListener.jobs[p] = ch
	}

	return nil
}

// manageUnitSync synchronously manages units
func (c *Connector) manageUnitSync(unitName string, method string, ctx context.Context) (bool, error) {
	ch := make(chan string)
	defer close(ch)

	err := c.manageUnits(unitName, method, ctx, ch)

	if err != nil {
		return false, err
	}

	// Wait for the result
	return <-ch == JobResultDone, nil
}

// ReqStartUnit performs a start and offers to pass on the data to the callback channel
func (c *Connector) ReqStartUnit(unitName string, ctx context.Context, cb chan<- string) error {
	return c.manageUnits(unitName, BusInterfaceStartUnit, ctx, cb)
}

// ReqStopUnit performs a stop and offers to pass on the data to the callback channel
func (c *Connector) ReqStopUnit(unitName string, ctx context.Context, cb chan<- string) error {
	return c.manageUnits(unitName, BusInterfaceStopUnit, ctx, cb)
}

// ReqRestartUnit performs a restart and offers to pass on the data to the callback channel
func (c *Connector) ReqRestartUnit(unitName string, ctx context.Context, cb chan<- string) error {
	return c.manageUnits(unitName, BusInterfaceRestartUnit, ctx, cb)
}

// RestartUnit synchronously restarts an unit and returns true if it succeded
func (c *Connector) RestartUnit(unitName string, ctx context.Context) (bool, error) {
	return c.manageUnitSync(unitName, BusInterfaceRestartUnit, ctx)
}

// StopUnit synchronously stops an unit and returns true if it succeded
func (c *Connector) StopUnit(unitName string, ctx context.Context) (bool, error) {
	return c.manageUnitSync(unitName, BusInterfaceStopUnit, ctx)
}

// StartUnit synchronously starts an unit and returns true if it succeded
func (c *Connector) StartUnit(unitName string, ctx context.Context) (bool, error) {
	return c.manageUnitSync(unitName, BusInterfaceStartUnit, ctx)
}

func getUnitObjectPath(unitName string) dbus.ObjectPath {
	return dbus.ObjectPath(BusObjectSystemdPath + "/unit/" + EscapeObjectPath(unitName))
}

// Retrieves the unit state for a specified unit
func (c *Connector) CheckUnitState(unitName string, ctx context.Context) (string, error) {
	if !c.Connected() {
		return "", &dbuscon.NotConnectedError{}
	}

	// Grab the connection
	conn := c.client.GetConnection()
	unit := conn.Object(BusObjectSystemdDest, getUnitObjectPath(unitName))

	var state string
	err := unit.CallWithContext(ctx, BusMemberGetProp, 0,
		BusObjectSystemdDestUnit, BusObjectPropertyActiveState).Store(&state)

	return state, err
}

// Signal registers a signal channel
func (c *Connector) Signal(ch chan<- *dbus.Signal) error {
	if !c.Connected() {
		return &dbuscon.NotConnectedError{}
	}

	c.client.GetConnection().Signal(ch)
	return nil
}

// RemoveSignal removes a signal channel from the list
func (c *Connector) RemoveSignal(ch chan<- *dbus.Signal) error {
	if !c.Connected() {
		return &dbuscon.NotConnectedError{}
	}

	c.client.GetConnection().RemoveSignal(ch)
	return nil
}

// Connected returns if the client is correctly connected
func (c *Connector) Connected() bool {
	_, ok := c.client.Connected()
	return ok
}

// AddMatchSignal adds a match signal
func (c *Connector) AddMatchSignal(options ...dbus.MatchOption) error {
	if !c.Connected() {
		return nil
	}

	return c.client.GetConnection().AddMatchSignal(options...)
}

// RemoveMatchSignal removes a match signal
func (c *Connector) RemoveMatchSignal(options ...dbus.MatchOption) error {
	if !c.Connected() {
		return nil
	}

	return c.client.GetConnection().RemoveMatchSignal(options...)
}

// This function requests a reconnect
func (c *Connector) RequestReconnect() error {
	c.Lock()
	defer c.Unlock()

	if c.Connected() {
		return fmt.Errorf("still connected and working")
	}

	log.Info("shutting down systemd connector (restart pending)")
	err := c.Shutdown()
	log.Info("shut down complete", zap.Error(err))

	err = c.init()

	return err
}

// This go-routine listens for signals
func (c *Connector) listenForSignals() {
	go func() {
		for {
			signal, ok := <-c.signalCh
			if !ok {
				log.Debug("signal channel terminated")
				return
			}

			// If its a job removed signal, our job terminated
			if signal.Name == BusSignalJobRemoved {
				c.jobCompleteSignal(signal)
			}
		}
	}()
}
