//go:build windows

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestReplaceBinaryWindowsDeferred(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "outlookctl.exe")
	next := filepath.Join(t.TempDir(), "outlookctl.new.exe")
	backup := filepath.Join(dir, "outlookctl.exe.bak")
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("cmd.exe", "/c", "exit", "0")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pid := child.Process.Pid
	_ = child.Wait()
	if err := replaceBinaryForProcess(current, next, backup, pid); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		currentData, currentErr := os.ReadFile(current)
		backupData, backupErr := os.ReadFile(backup)
		if currentErr == nil && backupErr == nil && string(currentData) == "new" && string(backupData) == "old" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	currentData, _ := os.ReadFile(current)
	backupData, _ := os.ReadFile(backup)
	t.Fatalf("deferred replace timeout: current=%q backup=%q", currentData, backupData)
}
