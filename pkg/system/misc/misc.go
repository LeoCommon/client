package misc

import (
	"encoding/binary"
	"fmt"
	"unsafe"
)

// Get proper binary endianess handler
func NativeEndianess() binary.ByteOrder {
	var x uint32 = 0x01020304
	if *(*byte)(unsafe.Pointer(&x)) == 0x01 {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

func BoolPointer(b bool) *bool {
	return &b
}

const (
	StateOFF = "off"
	StateON  = "on"
)

func ParseOnOffState(state string) (*bool, error) {
	if state == StateON {
		return BoolPointer(true), nil
	}

	if state == StateOFF {
		return BoolPointer(false), nil
	}

	return nil, fmt.Errorf("state was neither on nor off, got %v", state)
}
