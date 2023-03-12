package usb

import (
	"os/exec"
	"strconv"

	"github.com/google/gousb"
)

type DeviceType int

const (
	Unknown DeviceType = iota
	// SDRs
	SDRHackRFOne
	SDRHackRFJawbreaker
	// Modems
	ModemSIM7600
)

var (
	SupportedDevices = DeviceMap{
		SDRHackRFOne: {
			VendorID:  0x1d50,
			ProductID: 0x6089,
			Name:      "HackRFOne",
			ResetCMD:  exec.Command("hackrf_spiflash", "-R"),
		},
		SDRHackRFJawbreaker: {
			VendorID:  0x1d50,
			ProductID: 0x604b,
			Name:      "HackRFJawbreaker",
			ResetCMD:  exec.Command("hackrf_spiflash", "-R"),
		},
		ModemSIM7600: {
			VendorID:  0x1e0e,
			ProductID: 0x9001,
			Name:      "Simtech SIM7[5|6]00",
		},
	}
)

type Device struct {
	ResetCMD  *exec.Cmd
	Name      string
	VendorID  gousb.ID
	ProductID gousb.ID
}

type DeviceMap map[DeviceType]*Device

type DeviceTuple struct {
	*Device
	DeviceType
}

func FindSupportedDeviceTuple(vendorID gousb.ID, productID gousb.ID) (DeviceTuple, bool) {
	for k, device := range SupportedDevices {
		if device.VendorID == vendorID && device.ProductID == productID {
			return DeviceTuple{DeviceType: k, Device: device}, true
		}
	}
	return DeviceTuple{}, false
}

func ParseHexUINT16(str string) (uint16, error) {
	val, err := strconv.ParseUint(str, 16, 16)
	if err != nil {
		return 0, err
	}

	return uint16(val), nil
}
