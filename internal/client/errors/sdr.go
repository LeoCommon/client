package errors

type SDRNotFoundError struct {
	msg string
}

func (m *SDRNotFoundError) Error() string {
	return m.msg
}

func (e *SDRNotFoundError) Is(tgt error) bool {
	_, ok := tgt.(*SDRNotFoundError)
	return ok
}

type SDRStuckError struct {
	msg string
}

func (m *SDRStuckError) Error() string {
	return m.msg
}

func (e *SDRStuckError) Is(tgt error) bool {
	_, ok := tgt.(*SDRStuckError)
	return ok
}

// Generic TimedOutError
type TimedOutError struct {
	msg string
}

func (m *TimedOutError) Error() string {
	return m.msg
}

func (e *TimedOutError) Is(tgt error) bool {
	_, ok := tgt.(*TimedOutError)
	return ok
}

func NewTerminatedEarlyError(err error) error {
	return &TerminatedEarlyError{err}
}

type TerminatedEarlyError struct {
	err error
}

func (m *TerminatedEarlyError) Error() string {
	if m.err == nil {
		return "no underlying error, exited fine"
	}

	return m.err.Error()
}

func (e *TerminatedEarlyError) Is(tgt error) bool {
	_, ok := tgt.(*TerminatedEarlyError)
	return ok
}
