package systemd

import (
	"fmt"
	"strings"
	"unicode"
)

// EscapeObjectPath sanitizes a D-Bus ObjectPath string.
// This function replaces any characters that are not letters or digits with their
// corresponding hexadecimal encoding, preceded by an underscore character.
// If the input string is empty, this function returns an underscore character.
func EscapeObjectPath(path string) string {
	// Empty string is just _
	if len(path) == 0 {
		return "_"
	}

	var b strings.Builder
	for i, c := range path {
		if mustEscape(i, c) {
			fmt.Fprintf(&b, "_%x", c)
		} else {
			b.WriteRune(c)
		}
	}

	return b.String()
}

// mustEscape checks whether a rune in a potential dbus ObjectPath must be escaped.
// A rune must be escaped if it is not a letter or digit, or if it is the first
// character of the path and it is a digit.
func mustEscape(i int, c rune) bool {
	return (i == 0 && unicode.IsDigit(c)) || (!unicode.IsLetter(c) && !unicode.IsDigit(c))
}
