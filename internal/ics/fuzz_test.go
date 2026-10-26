package ics

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:1\r\nSUMMARY:test\r\nDTSTART:20261101T090000Z\r\nDTEND:20261101T100000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = Parse(strings.NewReader(input))
	})
}
