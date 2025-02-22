package streamhelpers

import (
	"bufio"
	"io"
	"sync"

	"github.com/LeoCommon/client/pkg/log"
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

type ConcurrentBufioWriter struct {
	sync.Mutex
	bufioWriter *bufio.Writer
}

func (w *ConcurrentBufioWriter) WriteString(s string) (int, error) {
	w.Lock()
	defer w.Unlock()
	return w.bufioWriter.WriteString(s)
}

func (w *ConcurrentBufioWriter) Write(b []byte) (int, error) {
	w.Lock()
	defer w.Unlock()
	return w.bufioWriter.Write(b)
}

func (w *ConcurrentBufioWriter) Flush() {
	w.Lock()
	defer w.Unlock()
	w.bufioWriter.Flush()
}
