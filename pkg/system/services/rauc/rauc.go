package rauc

import (
	"github.com/LeoCommon/client/pkg/systemd"
	"github.com/LeoCommon/client/pkg/systemd/dbuscon"
)

type SlotStatusType string

const (
	SlotStatusGood       SlotStatusType = "good"
	SlotStatusBad        SlotStatusType = "bad"
	MarkedSlotIdentifier string         = "booted"
)

func (c SlotStatusType) String() string {
	return string(c)
}

// Service interface methods
type Service interface {
	MarkBooted(status SlotStatusType) (slotName string, err error)
	SlotStatiString() string
	initialize() error
	Shutdown()
}

func NewService(sysdc *systemd.Connector) (Service, error) {
	if sysdc == nil || !sysdc.Connected() {
		return nil, &dbuscon.NotConnectedError{}
	}

	e := &raucDbusService{conn: sysdc.GetRawDbusConnection()}
	return e, e.initialize()
}
