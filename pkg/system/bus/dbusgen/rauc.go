package dbusgen

import (
	"context"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// Signal is a common interface for all signals.
type Signal interface {
	Name() string
	Interface() string
	Sender() string

	path() dbus.ObjectPath
	values() []interface{}
}

// Emit sends the given signal to the bus.
func Emit(conn *dbus.Conn, s Signal) error {
	return conn.Emit(s.path(), s.Interface()+"."+s.Name(), s.values()...)
}

// ErrUnknownSignal is returned by LookupSignal when a signal cannot be resolved.
var ErrUnknownSignal = errors.New("unknown signal")

// LookupSignal converts the given raw D-Bus signal with variable body
// into one with typed structured body or returns ErrUnknownSignal error.
func LookupSignal(signal *dbus.Signal) (Signal, error) {
	switch signal.Name {
	case InterfaceDe_Pengutronix_Rauc_Installer + "." + "Completed":
		v0, ok := signal.Body[0].(int32)
		if !ok {
			return nil, fmt.Errorf("prop .Result is %T, not int32", signal.Body[0])
		}
		return &De_Pengutronix_Rauc_Installer_CompletedSignal{
			sender: signal.Sender,
			Path:   signal.Path,
			Body: &De_Pengutronix_Rauc_Installer_CompletedSignalBody{
				Result: v0,
			},
		}, nil
	default:
		return nil, ErrUnknownSignal
	}
}

// AddMatchSignal registers a match rule for the given signal,
// opts are appended to the automatically generated signal's rules.
func AddMatchSignal(conn *dbus.Conn, s Signal, opts ...dbus.MatchOption) error {
	return conn.AddMatchSignal(append([]dbus.MatchOption{
		dbus.WithMatchInterface(s.Interface()),
		dbus.WithMatchMember(s.Name()),
	}, opts...)...)
}

// RemoveMatchSignal unregisters the previously registered subscription.
func RemoveMatchSignal(conn *dbus.Conn, s Signal, opts ...dbus.MatchOption) error {
	return conn.RemoveMatchSignal(append([]dbus.MatchOption{
		dbus.WithMatchInterface(s.Interface()),
		dbus.WithMatchMember(s.Name()),
	}, opts...)...)
}

// Interface name constants.
const (
	InterfaceDe_Pengutronix_Rauc_Installer = "de.pengutronix.rauc.Installer"
)

// De_Pengutronix_Rauc_Installerer is de.pengutronix.rauc.Installer interface.
type De_Pengutronix_Rauc_Installerer interface {
	// Install is de.pengutronix.rauc.Installer.Install method.
	Install(source string) (err *dbus.Error)
	// InstallBundle is de.pengutronix.rauc.Installer.InstallBundle method.
	InstallBundle(source string, args map[string]dbus.Variant) (err *dbus.Error)
	// Info is de.pengutronix.rauc.Installer.Info method.
	Info(bundle string) (compatible string, version string, err *dbus.Error)
	// Mark is de.pengutronix.rauc.Installer.Mark method.
	Mark(state string, slotIdentifier string) (slotName string, message string, err *dbus.Error)
	// GetSlotStatus is de.pengutronix.rauc.Installer.GetSlotStatus method.
	GetSlotStatus() (slotStatusArray []struct {
		V1 map[string]dbus.Variant
		V0 string
	}, err *dbus.Error)
	// GetPrimary is de.pengutronix.rauc.Installer.GetPrimary method.
	GetPrimary() (primary string, err *dbus.Error)
}

// ExportDe_Pengutronix_Rauc_Installer exports the given object that implements de.pengutronix.rauc.Installer on the bus.
func ExportDe_Pengutronix_Rauc_Installer(conn *dbus.Conn, path dbus.ObjectPath, v De_Pengutronix_Rauc_Installerer) error {
	return conn.ExportSubtreeMethodTable(map[string]interface{}{
		"Install":       v.Install,
		"InstallBundle": v.InstallBundle,
		"Info":          v.Info,
		"Mark":          v.Mark,
		"GetSlotStatus": v.GetSlotStatus,
		"GetPrimary":    v.GetPrimary,
	}, path, InterfaceDe_Pengutronix_Rauc_Installer)
}

// UnexportDe_Pengutronix_Rauc_Installer unexports de.pengutronix.rauc.Installer interface on the named path.
func UnexportDe_Pengutronix_Rauc_Installer(conn *dbus.Conn, path dbus.ObjectPath) error {
	return conn.Export(nil, path, InterfaceDe_Pengutronix_Rauc_Installer)
}

// UnimplementedDe_Pengutronix_Rauc_Installer can be embedded to have forward compatible server implementations.
type UnimplementedDe_Pengutronix_Rauc_Installer struct{}

func (*UnimplementedDe_Pengutronix_Rauc_Installer) iface() string {
	return InterfaceDe_Pengutronix_Rauc_Installer
}

func (*UnimplementedDe_Pengutronix_Rauc_Installer) Install(source string) (err *dbus.Error) {
	err = &dbus.ErrMsgUnknownMethod
	return
}

func (*UnimplementedDe_Pengutronix_Rauc_Installer) InstallBundle(source string, args map[string]dbus.Variant) (err *dbus.Error) {
	err = &dbus.ErrMsgUnknownMethod
	return
}

func (*UnimplementedDe_Pengutronix_Rauc_Installer) Info(bundle string) (compatible string, version string, err *dbus.Error) {
	err = &dbus.ErrMsgUnknownMethod
	return
}

func (*UnimplementedDe_Pengutronix_Rauc_Installer) Mark(state string, slotIdentifier string) (slotName string, message string, err *dbus.Error) {
	err = &dbus.ErrMsgUnknownMethod
	return
}

func (*UnimplementedDe_Pengutronix_Rauc_Installer) GetSlotStatus() (slotStatusArray []struct {
	V0 string
	V1 map[string]dbus.Variant
}, err *dbus.Error) {
	err = &dbus.ErrMsgUnknownMethod
	return
}

func (*UnimplementedDe_Pengutronix_Rauc_Installer) GetPrimary() (primary string, err *dbus.Error) {
	err = &dbus.ErrMsgUnknownMethod
	return
}

// NewDe_Pengutronix_Rauc_Installer creates and allocates de.pengutronix.rauc.Installer.
func NewDe_Pengutronix_Rauc_Installer(object dbus.BusObject) *De_Pengutronix_Rauc_Installer {
	return &De_Pengutronix_Rauc_Installer{object}
}

// De_Pengutronix_Rauc_Installer implements de.pengutronix.rauc.Installer D-Bus interface.
type De_Pengutronix_Rauc_Installer struct {
	object dbus.BusObject
}

// Install calls de.pengutronix.rauc.Installer.Install method.
func (o *De_Pengutronix_Rauc_Installer) Install(ctx context.Context, source string) (err error) {
	err = o.object.CallWithContext(ctx, InterfaceDe_Pengutronix_Rauc_Installer+".Install", 0, source).Store()
	return
}

// InstallBundle calls de.pengutronix.rauc.Installer.InstallBundle method.
//
// Annotations:
//
//	@org.qtproject.QtDBus.QtTypeName.In1 = QVariantMap
func (o *De_Pengutronix_Rauc_Installer) InstallBundle(ctx context.Context, source string, args map[string]dbus.Variant) (err error) {
	err = o.object.CallWithContext(ctx, InterfaceDe_Pengutronix_Rauc_Installer+".InstallBundle", 0, source, args).Store()
	return
}

// Info calls de.pengutronix.rauc.Installer.Info method.
func (o *De_Pengutronix_Rauc_Installer) Info(ctx context.Context, bundle string) (compatible string, version string, err error) {
	err = o.object.CallWithContext(ctx, InterfaceDe_Pengutronix_Rauc_Installer+".Info", 0, bundle).Store(&compatible, &version)
	return
}

// Mark calls de.pengutronix.rauc.Installer.Mark method.
func (o *De_Pengutronix_Rauc_Installer) Mark(ctx context.Context, state string, slotIdentifier string) (slotName string, message string, err error) {
	err = o.object.CallWithContext(ctx, InterfaceDe_Pengutronix_Rauc_Installer+".Mark", 0, state, slotIdentifier).Store(&slotName, &message)
	return
}

// GetSlotStatus calls de.pengutronix.rauc.Installer.GetSlotStatus method.
//
// Annotations:
//
//	@org.qtproject.QtDBus.QtTypeName.Out0 = RaucSlotStatusArray
func (o *De_Pengutronix_Rauc_Installer) GetSlotStatus(ctx context.Context) (slotStatusArray []struct {
	V0 string
	V1 map[string]dbus.Variant
}, err error) {
	err = o.object.CallWithContext(ctx, InterfaceDe_Pengutronix_Rauc_Installer+".GetSlotStatus", 0).Store(&slotStatusArray)
	return
}

// GetPrimary calls de.pengutronix.rauc.Installer.GetPrimary method.
func (o *De_Pengutronix_Rauc_Installer) GetPrimary(ctx context.Context) (primary string, err error) {
	err = o.object.CallWithContext(ctx, InterfaceDe_Pengutronix_Rauc_Installer+".GetPrimary", 0).Store(&primary)
	return
}

// GetOperation gets de.pengutronix.rauc.Installer.Operation property.
func (o *De_Pengutronix_Rauc_Installer) GetOperation(ctx context.Context) (operation string, err error) {
	err = o.object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, InterfaceDe_Pengutronix_Rauc_Installer, "Operation").Store(&operation)
	return
}

// GetLastError gets de.pengutronix.rauc.Installer.LastError property.
func (o *De_Pengutronix_Rauc_Installer) GetLastError(ctx context.Context) (lastError string, err error) {
	err = o.object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, InterfaceDe_Pengutronix_Rauc_Installer, "LastError").Store(&lastError)
	return
}

// GetProgress gets de.pengutronix.rauc.Installer.Progress property.
//
// Annotations:
//
//	@org.qtproject.QtDBus.QtTypeName = RaucProgress
func (o *De_Pengutronix_Rauc_Installer) GetProgress(ctx context.Context) (progress struct {
	V0 int32
	V1 string
	V2 int32
}, err error) {
	err = o.object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, InterfaceDe_Pengutronix_Rauc_Installer, "Progress").Store(&progress)
	return
}

// GetCompatible gets de.pengutronix.rauc.Installer.Compatible property.
func (o *De_Pengutronix_Rauc_Installer) GetCompatible(ctx context.Context) (compatible string, err error) {
	err = o.object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, InterfaceDe_Pengutronix_Rauc_Installer, "Compatible").Store(&compatible)
	return
}

// GetVariant gets de.pengutronix.rauc.Installer.Variant property.
func (o *De_Pengutronix_Rauc_Installer) GetVariant(ctx context.Context) (variant string, err error) {
	err = o.object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, InterfaceDe_Pengutronix_Rauc_Installer, "Variant").Store(&variant)
	return
}

// GetBootSlot gets de.pengutronix.rauc.Installer.BootSlot property.
func (o *De_Pengutronix_Rauc_Installer) GetBootSlot(ctx context.Context) (bootSlot string, err error) {
	err = o.object.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, InterfaceDe_Pengutronix_Rauc_Installer, "BootSlot").Store(&bootSlot)
	return
}

// De_Pengutronix_Rauc_Installer_CompletedSignal represents de.pengutronix.rauc.Installer.Completed signal.
type De_Pengutronix_Rauc_Installer_CompletedSignal struct {
	sender string
	Path   dbus.ObjectPath
	Body   *De_Pengutronix_Rauc_Installer_CompletedSignalBody
}

// Name returns the signal's name.
func (s *De_Pengutronix_Rauc_Installer_CompletedSignal) Name() string {
	return "Completed"
}

// Interface returns the signal's interface.
func (s *De_Pengutronix_Rauc_Installer_CompletedSignal) Interface() string {
	return InterfaceDe_Pengutronix_Rauc_Installer
}

// Sender returns the signal's sender unique name.
func (s *De_Pengutronix_Rauc_Installer_CompletedSignal) Sender() string {
	return s.sender
}

func (s *De_Pengutronix_Rauc_Installer_CompletedSignal) path() dbus.ObjectPath {
	return s.Path
}

func (s *De_Pengutronix_Rauc_Installer_CompletedSignal) values() []interface{} {
	return []interface{}{s.Body.Result}
}

// De_Pengutronix_Rauc_Installer_CompletedSignalBody is body container.
type De_Pengutronix_Rauc_Installer_CompletedSignalBody struct {
	Result int32
}
