package audit

import (
	"encoding/json"
	"os"
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
