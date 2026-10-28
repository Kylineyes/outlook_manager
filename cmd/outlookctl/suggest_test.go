package main

import (
	"errors"
	"testing"
)

func TestSuggestCommand(t *testing.T) {
	if got := suggestCommand("mial"); got != "mail" {
		t.Fatalf("suggest mial = %q", got)
	}
	if got := suggestCommand("calender"); got != "calendar" {
		t.Fatalf("suggest calender = %q", got)
	}
}

func TestExitCodeError(t *testing.T) {
	err := withExitCode(2, errors.New("bad usage"))
	var coded interface{ ExitCode() int }
	if !errors.As(err, &coded) || coded.ExitCode() != 2 {
		t.Fatalf("unexpected exit code error: %#v", err)
	}
}
