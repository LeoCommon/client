package rauc

import (
	"context"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/system/dbusclient/dbusgen"
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

	apglog.Info("Marked slot", zap.String("slot", slot), zap.String("status", status.String()))
	return slot, err
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
