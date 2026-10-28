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

func TestApplyAndRollbackUnix(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "outlookctl")
	newBinary := filepath.Join(t.TempDir(), "outlookctl.new")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBinary, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	apply, err := ApplyDownloadTo(&DownloadResult{BinaryPath: newBinary}, exe)
	if err != nil {
		t.Fatal(err)
	}
	if apply.BackupPath == "" {
		t.Fatal("backup path is empty")
	}
	data, _ := os.ReadFile(exe)
	if string(data) != "new" {
		t.Fatalf("exe = %q", data)
	}
	if _, err := RollbackFrom(exe); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(exe)
	if string(data) != "old" {
		t.Fatalf("rolled back exe = %q", data)
	}
}
