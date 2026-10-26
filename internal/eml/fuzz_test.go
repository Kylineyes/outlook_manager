package eml

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add("From: a@example.com\r\nTo: b@example.com\r\nSubject: test\r\nContent-Type: text/plain\r\n\r\nhello\r\n")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = Parse(strings.NewReader(input))
	})
}
