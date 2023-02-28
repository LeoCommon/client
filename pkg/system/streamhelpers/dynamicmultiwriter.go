package streamhelpers

import (
	"io"
	"sync"
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

func (t *DynamicMultiWriter) Append(writers ...io.Writer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.writers = append(t.writers, writers...)
}

func (t *DynamicMultiWriter) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.writers)
}

type DynamicMultiWriter struct {
	writers []io.Writer
	mu      sync.RWMutex
}

func (t *DynamicMultiWriter) Write(p []byte) (n int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, w := range t.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

func (t *DynamicMultiWriter) WriteString(s string) (n int, err error) {
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
