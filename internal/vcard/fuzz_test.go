package vcard

import (
	"strings"
	"testing"
)

func FuzzReadContacts(f *testing.F) {
	f.Add("BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Alice\r\nEMAIL:a@example.com\r\nEND:VCARD\r\n")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ReadContacts(strings.NewReader(input))
	})
}
