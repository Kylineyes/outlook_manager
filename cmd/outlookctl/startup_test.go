package main

import (
	"reflect"
	"testing"
)

func TestStripGlobalUpdateFlag(t *testing.T) {
	args, disabled := stripGlobalUpdateFlag([]string{"mail", "list", "--no-update-check", "--top", "3"})
	if !disabled {
		t.Fatal("expected disabled")
	}
	want := []string{"mail", "list", "--top", "3"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}
