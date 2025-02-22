package usb

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/DiscoResearchSat/go-udev/netlink"
	"github.com/LeoCommon/client/pkg/log"
	"github.com/google/gousb"
	"go.uber.org/zap"
)

func (d *Device) String() string {
	return fmt.Sprintf("%s pid: %s vid: %s", d.Name, d.ProductID.String(), d.VendorID.String())
}

type USBDeviceManager struct {
	sync.Mutex
	sync.WaitGroup

	// A map of currently connected devices
	devices DeviceMap
	// Channel to close the udev monitor if its enabled
	udevCloseChannel chan struct{}
	// The udev event connection, if not nil, udev monitoring is active
	udev *netlink.UEventConn
}

func NewUSBDeviceManager() *USBDeviceManager {
	m := &USBDeviceManager{
		devices:          make(DeviceMap),
		udev:             new(netlink.UEventConn),
		udevCloseChannel: make(chan struct{}),
	}

	// Connect to udev
	if err := m.udev.Connect(netlink.UdevEvent); err != nil {
		log.Error("Could not connect to udev, hotplug support not available!", zap.Error(err))
		m.udev = nil
	} else {
		// run monitor
		m.Add(1)
		go m.monitor()
	}

	return m
}

func (m *USBDeviceManager) FindSupportedDevices() DeviceMap {
	m.Lock()
	defer m.Unlock()

	usbCtx := gousb.NewContext()
	for supSDR, d := range SupportedDevices {
		dev, err := usbCtx.OpenDeviceWithVIDPID(d.VendorID, d.ProductID)
		if dev == nil {
			log.Debug("device not attached", zap.String("sdr", d.String()))
			continue
		}

		// close the device
		dev.Close()

		if err != nil {
			log.Error("error while iterating over usb devices", zap.Error(err))
			continue
		}

		// Add the device to the found devices
		m.devices[supSDR] = d
		log.Info("found supported device", zap.String("sdr", d.String()))
	}
	usbCtx.Close()

	return m.devices
}

func (m *USBDeviceManager) HotplugReceived(VendorID uint16, productID uint16, wasAdded bool) {
	m.Lock()
	defer m.Unlock()

	// Try to find device, silently ignore if not supported
	tuple, found := FindSupportedDeviceTuple(gousb.ID(VendorID), gousb.ID(productID))
	if !found {
		log.Debug("no matching device found", zap.String("vid", gousb.ID(VendorID).String()), zap.String("pid", gousb.ID(productID).String()))
		return
	}

	// No further checks, no duplicates as key is unique
	if !wasAdded {
		delete(m.devices, tuple.DeviceType)
		log.Info("hotplug device removed", zap.String("device", tuple.Device.String()))
	} else {
		log.Info("hotplug device added", zap.String("device", tuple.Device.String()))
		m.devices[tuple.DeviceType] = tuple.Device
	}
}

func (m *USBDeviceManager) Shutdown() {
	m.Lock()

	// Close the udev monitor if it exists
	if m.udev != nil {
		log.Info("closing udev monitor channel")
		m.udevCloseChannel <- struct{}{}
	}

	m.Unlock()
	m.Wait()
}

func (m *USBDeviceManager) ResetDevice(target DeviceType) error {
	m.Lock()
	defer m.Unlock()

	// Grab the details for more descriptive errors
	supd, exists := SupportedDevices[target]
	if !exists {
		return fmt.Errorf("device unknown, add it to the code")
	}

	d, exists := m.devices[target]
	if !exists {
		return NewNotFoundError(fmt.Sprintf("device with Name '%s' not attached", supd.Name))
	}

	// If the device has a dedicated reset CMD, try it first
	if d.ResetCMD != nil {
		err := d.ResetCMD.Run()
		if err == nil {
			log.Info("device reset cmd executed", zap.String("device", d.String()))
			return nil
		} else {
			log.Error("device reset with cmd failed, continuing", zap.String("device", d.String()), zap.Error(err))
		}
	}

	// Try acquiring the device and issuing a simple usb reset
	usbCtx := gousb.NewContext()
	defer usbCtx.Close()
	dev, _ := usbCtx.OpenDeviceWithVIDPID(d.VendorID, d.ProductID)
	if dev == nil {
		log.Error("the device was detected previously, but disappeared!", zap.String("device", d.String()))
		return NewVanishedError(fmt.Sprintf("%s disapperared but was detected before", d.String()))
	}

	// Close when we are done
	defer dev.Close()

	if err := dev.Reset(); err != nil {
		log.Error("resetting usb device failed", zap.String("device", d.String()))
		return err
	}

	// Note: Do not delete the device an usb reset does not trigger any udev rules.
	// todo: prepare a signal to fire that the reset completed if the supported device list indicates that a hotplug will take place.
	return nil
}

// Monitor events
func (m *USBDeviceManager) monitor() {
	errors := make(chan error)

	// BIND OR UNBIND
	matchRule := fmt.Sprintf("%s|%s", netlink.BIND, netlink.UNBIND)
	deviceMatcher := &netlink.RuleDefinitions{
		Rules: []netlink.RuleDefinition{
			{
				// Only match usb_device binds and unbinds
				Action: &matchRule,
				Env: map[string]string{
					"DEVTYPE": "usb_device",
				},
			},
		},
	}

	// Start he monitor
	ctx, cancelUdevMonitor := context.WithCancel(context.Background())
	queue := m.udev.Monitor(ctx, errors, deviceMatcher)

	// Defer the channel closing and marking wg as done
	defer func() {
		m.Lock()
		m.udev.Close()
		m.Done()
		m.Unlock()
	}()

udevMonitorLoop:
	for {
		select {
		case <-m.udevCloseChannel:
			// Wait until the queue terminates
			cancelUdevMonitor()
			// Wait for context-cancelled error
			<-errors
			break udevMonitorLoop

		case uevent := <-queue:
			// log.Debug("event", zap.Any("ev", uevent.Env))
			pstr, pok := uevent.Env["PRODUCT"]
			if !pok {
				log.Debug("device did not contain product indicator", zap.Any("env", uevent.String()))
				continue
			}

			// Split at "/" e.g "PRODUCT":"1d50/6089/104" VID/PID/REVISION
			s := strings.Split(pstr, "/")
			if len(s) < 2 {
				log.Error("malformed product string", zap.String("product", pstr))
				continue
			}

			vidStr := s[0]
			pidStr := s[1]

			// Check for empty string
			var vid, pid uint16
			var err error
			if vid, err = ParseHexUINT16(vidStr); err != nil {
				log.Error("could not parse hex vid", zap.String("vidStr", vidStr))
				continue
			}
			if pid, err = ParseHexUINT16(pidStr); err != nil {
				log.Error("could not parse hex pid", zap.String("pidStr", pidStr))
				continue
			}

			// log.Debug("got event", zap.String("vid", vidStr), zap.String("pid", pidStr))

			// Forward the event to the usb matcher
			m.HotplugReceived(vid, pid, uevent.Action == netlink.BIND)
		case err := <-errors:
			// I never saw this happen, not sure what to do here
			log.Error("udev monitor encountered an error", zap.Error(err))
		}
	}

	log.Info("stopped observing udev events")
}
