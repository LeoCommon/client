package streamhelpers

import (
	"io"
	"sync"
	"time"
)

// Taken and modified from the original multi.go source for io.MultiReader
func (t *DynamicMultiWriter) Remove(writer io.Writer) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i, v := range t.writers {
		if v == writer {
			t.writers = append(t.writers[:i], t.writers[i+1:]...)
			return true
		}
	}
	return false
}

func (t *DynamicMultiWriter) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.writers = []io.Writer{}
}

func (t *DynamicMultiWriter) Append(writers ...io.Writer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t._append(writers...)
}

func (t *DynamicMultiWriter) _append(writers ...io.Writer) {
	t.writers = append(t.writers, writers...)
}

// This code tries to append a Writer to the running code, it will return false if the request timed out
func (t *DynamicMultiWriter) RequestAppend(writer io.Writer, timeout time.Duration) bool {
	for {
		if t.mu.TryLock() {
			defer t.mu.Unlock()
			t._append(writer)
			return true
		}

		select {
		// try for one second, if we dont succeed call it a fail
		case <-time.After(timeout):
			return false
		default:
			// do nothing, just try again
		}
	}
}

func (t *DynamicMultiWriter) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.writers)
}

type DynamicMultiWriter struct {
	mu      sync.RWMutex
	writers []io.Writer
}

func (t *DynamicMultiWriter) Write(p []byte) (n int, err error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, w := range t.writers {
		// This call blocks indefinetly, so we might be stuck here, make sure the code outside can handle this!
		n, err = w.Write(p)
		if err != nil {
			return
		}
	}
	return len(p), nil
}

func (t *DynamicMultiWriter) WriteString(s string) (n int, err error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var p []byte // lazily initialized if/when needed
	for _, w := range t.writers {
		if sw, ok := w.(io.StringWriter); ok {
			n, err = sw.WriteString(s)
		} else {
			if p == nil {
				p = []byte(s)
			}
			n, err = w.Write(p)
		}
		if err != nil {
			return
		}
		if n != len(s) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(s), nil
}

// Creates a dynamic multiwriter, that is able to append and remove elements on the fly
// Implementation is close/identical to io.MultiWriter
func NewDynamicMultiWriter(writers ...io.Writer) *DynamicMultiWriter {
	w := make([]io.Writer, len(writers))
	copy(w, writers)
	return &DynamicMultiWriter{writers: w}
}
