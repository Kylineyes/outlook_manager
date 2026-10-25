package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

func TestLogWritesJSONLine(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
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
	if entry["action"] != "test.action" || entry["id"] != "123" {
		t.Fatalf("unexpected audit entry: %#v", entry)
	}
	if entry["time"] == "" {
		t.Fatal("audit entry missing time")
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

func TestLogRotatesWhenSizeExceeded(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	oldSize := maxAuditLogSize
	maxAuditLogSize = 10
	defer func() { maxAuditLogSize = oldSize }()

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
