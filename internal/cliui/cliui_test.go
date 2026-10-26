package cliui

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintTable(t *testing.T) {
	var buffer bytes.Buffer
	if err := PrintTable(&buffer, []string{"Name", "Value"}, [][]string{{"a", "1"}, {"long", "2"}}); err != nil {
		t.Fatal(err)
	}
	output := buffer.String()
	if !strings.Contains(output, "Name") || !strings.Contains(output, "long") {
		t.Fatalf("unexpected table: %s", output)
	}
}
