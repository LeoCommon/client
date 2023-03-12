package misc

import (
	"fmt"
	"time"
)

// TimedOutError Generic error for timeouts
type TimedOutError struct {
	msg   string
	after time.Duration
}

func (t *TimedOutError) Error() string {
	return fmt.Sprintf("%s after %s", t.msg, t.after)
}

func (t *TimedOutError) Is(e error) bool {
	_, ok := e.(*TimedOutError)
	return ok
}

func NewTimedOutError(msg string, after time.Duration) error {
	return &TimedOutError{msg, after}
}
