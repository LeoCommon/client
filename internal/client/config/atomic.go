package config

import (
	"strconv"

	"go.uber.org/atomic"
)

// This file is marshaling and unmarshaling certain atomic non-string builtins so we can use them within toml
// This does not use native toml types, and yes this is due to pelletier/go-toml forcibly wrapping MarshalText with quotes

// Wrap the atomic boolean for automatic Marshal/Unmarshal, it also supports the native bool = false|true without quotes
// removeme this is more of a proof of concept, its cleaner to just use a locking mutex in these cases
type AtomicBoolean struct {
	b *atomic.Bool
}

func (x *AtomicBoolean) MarshalText() ([]byte, error) {
	if x.b.Load() {
		return []byte{'t', 'r', 'u', 'e'}, nil
	}

	return []byte{'f', 'a', 'l', 's', 'e'}, nil
}

func (x *AtomicBoolean) UnmarshalText(in []byte) error {
	b, err := strconv.ParseBool(string(in))
	if err != nil {
		return err
	}

	x.b = atomic.NewBool(b)
	return nil
}

func (x *AtomicBoolean) Store(value bool) {
	x.b.Store(value)
}

func (x *AtomicBoolean) Load() bool {
	return x.b.Load()
}
