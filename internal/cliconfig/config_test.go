package cliconfig

import (
	"os"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := Save(Settings{Output: "table", Color: "never", AutoUpdateCheck: true}); err != nil {
		t.Fatal(err)
	}
	settings := Load()
	if settings.Output != "table" || settings.Color != "never" || !settings.AutoUpdateCheck {
		t.Fatalf("unexpected settings: %#v", settings)
	}
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
