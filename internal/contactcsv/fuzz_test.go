package contactcsv

import (
	"strings"
	"testing"
)

func FuzzReadContacts(f *testing.F) {
	f.Add(strings.Join(headers, ",") + "\nAlice,a@example.com\n")
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = ReadContacts(strings.NewReader(input))
	})
}
