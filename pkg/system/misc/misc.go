package misc

import (
	"encoding/binary"
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
