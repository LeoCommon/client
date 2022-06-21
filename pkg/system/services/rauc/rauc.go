package rauc

import "github.com/godbus/dbus/v5"

type SlotStatusType string

const (
	SLOT_STATUS_GOOD       SlotStatusType = "good"
	SLOT_STATUS_BAD        SlotStatusType = "bad"
	MARKED_SLOT_IDENTIFIER string         = "booted"
)

func (c SlotStatusType) String() string {
	return string(c)
}

// # interface methods
type RaucService interface {
	MarkBooted(status SlotStatusType) (slotName string, err error)
}

func NewService(conn *dbus.Conn) (RaucService, error) {
	// #todo First try dbus, if fails cli
	e := &raucDbusService{conn: conn}
	e.initialize()

	return e, nil
}
