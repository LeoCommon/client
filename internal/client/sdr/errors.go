package sdr

type NotFoundError struct {
	msg string
}

func (m *NotFoundError) Error() string {
	return m.msg
}

func (e *NotFoundError) Is(tgt error) bool {
	_, ok := tgt.(*NotFoundError)
	return ok
}

type StuckError struct {
	msg string
}

func (m *StuckError) Error() string {
	return m.msg
}

func (e *StuckError) Is(tgt error) bool {
	_, ok := tgt.(*StuckError)
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
