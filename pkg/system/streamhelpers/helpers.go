package streamhelpers

import (
	"io"

	"disco.cs.uni-kl.de/apogee/pkg/log"
)

type CloseFunc func(err *error) error

var EmptyCloseFunc CloseFunc = func(err *error) error { return nil }

type CloseFuncPtr *CloseFunc
type CloseFuncPointers []CloseFuncPtr

func CloseIfCloseable(c interface{}) error {
	if cls, ok := c.(io.Closer); ok {
		log.Debug("closing stream that was closeable")
		return cls.Close()
	}

	// Undefined behavior, if it wasnt closeable we have to return nil
	return nil
}

func RemoveFromSlice[T comparable](l []T, item T) []T {
	for i, other := range l {
		if other == item {
			return append(l[:i], l[i+1:]...)
		}
	}
	return l
}
