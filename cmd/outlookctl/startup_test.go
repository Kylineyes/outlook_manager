package main

import (
	"reflect"
	"testing"

	"github.com/Kylineyes/outlook-manager/internal/cliconfig"
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

func TestResolveAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := cliconfig.Save(cliconfig.Settings{Output: "text", Color: "auto", Aliases: map[string]string{"m": "mail list", "unread": "mail query --unread"}}); err != nil {
		t.Fatal(err)
	}
	got := resolveAlias([]string{"m", "--top", "3"})
	want := []string{"mail", "list", "--top", "3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	original := []string{"calendar", "list"}
	if got := resolveAlias(original); !reflect.DeepEqual(got, original) {
		t.Fatalf("unexpected passthrough: %#v", got)
	}
}
