package streamhelpers

import (
	"io"
)

var ErrNilFunc = func() error { return nil }

func CloseIfCloseable(c interface{}) error {
	if cls, ok := c.(io.Closer); ok {
		return cls.Close()
	}

	// Undefined behavior
	return nil
}
