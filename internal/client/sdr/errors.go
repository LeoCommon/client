package sdr

type NotFoundError struct {
	msg string
}

func (n *NotFoundError) Error() string {
	return n.msg
}

func (n *NotFoundError) Is(e error) bool {
	_, ok := e.(*NotFoundError)
	return ok
}

type StuckError struct {
	msg string
}

func (s *StuckError) Error() string {
	return s.msg
}

func (s *StuckError) Is(e error) bool {
	_, ok := e.(*StuckError)
	return ok
}

// TimedOutError Generic error for timeouts
type TimedOutError struct {
	msg string
}

func (t *TimedOutError) Error() string {
	return t.msg
}

func (t *TimedOutError) Is(e error) bool {
	_, ok := e.(*TimedOutError)
	return ok
}
