//go:build !windows

package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceBinaryUnix(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "outlookctl")
	nextDir := t.TempDir()
	next := filepath.Join(nextDir, "outlookctl.new")
	backup := filepath.Join(dir, "outlookctl.bak")
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(current, next, backup); err != nil {
		t.Fatal(err)
	}
	currentData, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	backupData, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentData) != "new" || string(backupData) != "old" {
		t.Fatalf("current=%q backup=%q", currentData, backupData)
	}
}
