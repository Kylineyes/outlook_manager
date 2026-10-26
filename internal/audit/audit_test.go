package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

func TestLogWritesJSONLine(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	SetAccount("a@example.com")
	t.Cleanup(ClearAccount)
	Log("test.action", map[string]any{"id": "123", "count": 1})
	path, err := store.AuditLogPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry map[string]any
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("audit log is not valid JSON: %v", err)
	}
	if entry["action"] != "test.action" || entry["id"] != "123" || entry["account"] != "a@example.com" {
		t.Fatalf("unexpected audit entry: %#v", entry)
	}
	if entry["time"] == "" || entry["hash"] == "" {
		t.Fatalf("audit entry missing time/hash: %#v", entry)
	}
}

func TestEntriesFiltersAndLimits(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	Log("mail.send", map[string]any{"id": "1"})
	Log("calendar.create", map[string]any{"id": "2"})
	Log("mail.delete", map[string]any{"id": "3"})
	entries, err := Entries(10, "mail.")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	limited, err := Entries(1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0]["action"] != "mail.delete" {
		t.Fatalf("unexpected limited entries: %#v", limited)
	}
}

func TestQueryFiltersByAccountAndKeyword(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	SetAccount("a@example.com")
	Log("mail.send", map[string]any{"subject": "alpha"})
	SetAccount("b@example.com")
	Log("mail.send", map[string]any{"subject": "beta"})
	ClearAccount()
	entries, err := Query(QueryOptions{Account: "a@example.com", Query: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0]["account"] != "a@example.com" {
		t.Fatalf("unexpected query entries: %#v", entries)
	}
}

func TestExportAndStats(t *testing.T) {
	entries := []map[string]any{
		{"time": "2026-01-01T00:00:00Z", "action": "mail.send", "account": "a@example.com", "id": "1"},
		{"time": "2026-01-02T00:00:00Z", "action": "mail.delete", "account": "a@example.com", "id": "2"},
	}
	var csvBuffer bytes.Buffer
	if err := ExportCSV(&csvBuffer, entries); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csvBuffer.String(), "mail.send") || !strings.Contains(csvBuffer.String(), "account") {
		t.Fatalf("unexpected csv: %s", csvBuffer.String())
	}
	var jsonBuffer bytes.Buffer
	if err := ExportJSON(&jsonBuffer, entries); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jsonBuffer.String(), "mail.delete") {
		t.Fatalf("unexpected json: %s", jsonBuffer.String())
	}
	stats := BuildStats(entries)
	if stats.Total != 2 || stats.Actions["mail.send"] != 1 || stats.Accounts["a@example.com"] != 2 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestVerifyDetectsTamper(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	Log("mail.send", map[string]any{"id": "1"})
	Log("mail.delete", map[string]any{"id": "2"})
	result, err := Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Checked != 2 {
		t.Fatalf("unexpected verify result: %#v", result)
	}
	path, err := store.AuditLogPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "mail.send", "mail.sent", 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = Verify()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("tamper should be detected: %#v", result)
	}
}

func TestCleanupRetention(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	path, err := store.AuditLogPath()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(dir, "audit-20200101-000000.log")
	if err := os.WriteFile(oldFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	result, err := Cleanup(Settings{MaxSizeBytes: DefaultMaxSizeBytes, MaxBackups: 5, RetentionDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed == 0 {
		t.Fatal("expected old audit file removal")
	}
}

func TestLogRotatesWhenSizeExceeded(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	if err := SaveSettings(Settings{MaxSizeBytes: 10, MaxBackups: 5, RetentionDays: 30}); err != nil {
		t.Fatal(err)
	}
	Log("test.first", map[string]any{"id": "1"})
	Log("test.second", map[string]any{"id": "2"})
	path, err := store.AuditLogPath()
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	rotated := 0
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "audit-") && strings.HasSuffix(file.Name(), ".log") {
			rotated++
		}
	}
	if rotated == 0 {
		t.Fatal("expected rotated audit log")
	}
}

func TestConcurrentLog(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	const workers = 20
	const perWorker = 5
	done := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		go func(index int) {
			for j := 0; j < perWorker; j++ {
				Log("test.concurrent", map[string]any{"worker": index, "index": j})
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < workers; i++ {
		<-done
	}
	entries, err := Entries(0, "test.concurrent")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != workers*perWorker {
		t.Fatalf("entries = %d, want %d", len(entries), workers*perWorker)
	}
	result, err := Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Checked != workers*perWorker {
		t.Fatalf("unexpected verify result: %#v", result)
	}
}
