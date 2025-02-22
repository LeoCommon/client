package rauc

import (
	"context"

	"github.com/LeoCommon/client/pkg/log"
	"github.com/LeoCommon/client/pkg/systemd/dbuscon/dbusgen"
	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

const (
	DbusServiceDomain = "de.pengutronix.rauc"
	DbusObjectPath    = "/"
)

type raucDbusService struct {
	conn    *dbus.Conn
	service *dbusgen.De_Pengutronix_Rauc_Installer
}

// MarkBooted marks the currently booted slot
func (s *raucDbusService) MarkBooted(status SlotStatusType) (slotName string, err error) {
	slot, _, err := s.Mark(MarkedSlotIdentifier, status)
	if err != nil {
		log.Error("Could not mark slot with rauc", zap.String("error", err.Error()))
		return slot, err
	}

	log.Debug("Marked slot", zap.String("slot", slot), zap.String("status", status.String()))
	return slot, err
}

func (s *raucDbusService) Shutdown() {
	// empty stub
}

func (s *raucDbusService) SlotStatiString() string {
	//test, _ := s.service.GetSlotStatus(context.Background())
	// FIXME get real detailed status from this
	return "STUB"
}

func (s *raucDbusService) Mark(slotIdentifier string, status SlotStatusType) (slotName string, message string, err error) {
	return s.service.Mark(context.Background(), status.String(), slotIdentifier)
}

// GetBootSlot returns the booted slot in A/B format, not in rootfs.0!
func (s *raucDbusService) GetBootSlot() (string, error) {
	return s.service.GetBootSlot(context.Background())
}

// GetPrimary returns the primary slot
func (s *raucDbusService) GetPrimary() (string, error) {
	return s.service.GetPrimary(context.Background())
}

func (s *raucDbusService) initialize() error {
	s.service = dbusgen.NewDe_Pengutronix_Rauc_Installer(s.conn.Object(DbusServiceDomain, DbusObjectPath))
	return nil
}
