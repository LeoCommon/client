package usb

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

func NewNotFoundError(msg string) error {
	return &NotFoundError{msg}
}

type VanishedError struct {
	msg string
}

func (n *VanishedError) Error() string {
	return n.msg
}

func (n *VanishedError) Is(e error) bool {
	_, ok := e.(*VanishedError)
	return ok
}

func NewVanishedError(msg string) error {
	return &VanishedError{msg}
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

func NewStuckError(msg string) error {
	return &StuckError{msg}
}
