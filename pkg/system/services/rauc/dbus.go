package rauc

import (
	"context"
	"reflect"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/system/bus/dbusgen"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

const (
	RAUC_DBUS_SERVICE_DOMAIN = "de.pengutronix.rauc"
	RAUC_DBUS_OBJECT_PATH    = "/"
)

type raucDbusService struct {
	conn    *dbus.Conn
	service *dbusgen.De_Pengutronix_Rauc_Installer
}

// Marks the currently booted slot
func (s *raucDbusService) MarkBooted(status SlotStatusType) (slotName string, err error) {
	slot, _, err := s.Mark(MARKED_SLOT_IDENTIFIER, status)
	if err != nil {
		apglog.Error("Could not mark slot with rauc", zap.String("error", err.Error()))
		return slot, err
	}

	apglog.Debug("Marked slot", zap.String("slot", slot), zap.String("status", status.String()))
	return slot, err
}

func (s *raucDbusService) Shutdown() {
	// empty stub
}

// Ripped from https://stackoverflow.com/questions/39968236/how-to-convert-slice-of-structs-to-slice-of-strings-in-go
func GetFields(i interface{}) (res []string) {
	v := reflect.ValueOf(i)
	for j := 0; j < v.NumField(); j++ {
		res = append(res, v.Field(j).String())
	}
	return
}

// todo: No desire to work with this thing at the moment, so lets just make a string out of it
func (s *raucDbusService) SlotStatiString() string {
	stati, _ := s.service.GetSlotStatus(context.Background())

	var stringOutput []string
	for _, v := range stati {
		stringOutput = append(stringOutput, GetFields(v)...)
	}

	return strings.Join(stringOutput, ";")
}

func (s *raucDbusService) Mark(slotIdentifier string, status SlotStatusType) (slotName string, message string, err error) {
	return s.service.Mark(context.Background(), status.String(), slotIdentifier)
}

/* Does return the booted slot in A/B format, not in rootfs.0! */
func (s *raucDbusService) GetBootSlot() (string, error) {
	return s.service.GetBootSlot(context.Background())
}

/* Returns the primary slot */
func (s *raucDbusService) GetPrimary() (string, error) {
	return s.service.GetPrimary(context.Background())
}

func (s *raucDbusService) initialize() {
	s.service = dbusgen.NewDe_Pengutronix_Rauc_Installer(s.conn.Object(RAUC_DBUS_SERVICE_DOMAIN, RAUC_DBUS_OBJECT_PATH))
}
