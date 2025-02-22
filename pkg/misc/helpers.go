package misc

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"unsafe"

	"github.com/LeoCommon/client/pkg/log"
	"go.uber.org/zap"
)

// NativeEndianess Get proper binary endianess handler
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

func ParseFloat(inStr string, defVal float64, argument string) float64 {
	parsedValue, err := strconv.ParseFloat(inStr, 64)
	if err != nil {
		log.Warn("bad value",
			zap.String("argument", argument),
			zap.String("value", inStr),
		)
		return defVal
	}
	return parsedValue
}

func ParseInt(inStr string, defVal int64, argument string) int64 {
	parsedValue, err := strconv.ParseInt(inStr, 10, 64)
	if err != nil {
		log.Warn("bad value",
			zap.String("argument", argument),
			zap.String("value", inStr),
		)
		return defVal
	}
	return parsedValue
}
