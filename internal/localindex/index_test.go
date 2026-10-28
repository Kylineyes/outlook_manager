package localindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

func TestSQLiteSearchAndStats(t *testing.T) {
	setLocalIndexTestHome(t)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.replaceMessages("a@example.com", []graph.Message{
		{ID: "m1", Subject: "账单通知", BodyPreview: "本月账单", ReceivedDateTime: now, From: graph.MessageFrom{EmailAddress: graph.EmailAddress{Address: "billing@example.com"}}},
		{ID: "m2", Subject: "English invoice", BodyPreview: "payment due", ReceivedDateTime: now.Add(-time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.replaceContacts("a@example.com", []graph.Contact{{ID: "c1", DisplayName: "张三", EmailAddresses: []graph.ContactEmailAddress{{Address: "a@example.com"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.replaceEvents("a@example.com", []graph.CalendarEvent{{ID: "e1", Subject: "项目会议", Start: graph.DateTimeTimeZone{DateTime: "2026-11-01T09:00:00", TimeZone: "UTC"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	stats, err := LoadStats("a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Accounts) != 1 || stats.TotalMessages != 2 || stats.TotalContacts != 1 || stats.TotalEvents != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	hits, err := Search("a@example.com", "账单", "mail", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "m1" || hits[0].Account != "a@example.com" {
		t.Fatalf("unexpected hits: %#v", hits)
	}
	hits, err = Search("a@example.com", "invoice", "mail", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "m2" {
		t.Fatalf("unexpected FTS hits: %#v", hits)
	}
}

func TestMultiAccountIsolation(t *testing.T) {
	setLocalIndexTestHome(t)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.replaceMessages("a@example.com", []graph.Message{{ID: "a1", Subject: "account a"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.replaceMessages("b@example.com", []graph.Message{{ID: "b1", Subject: "account b"}}); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search("a@example.com", "account", "mail", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "a1" {
		t.Fatalf("unexpected account hits: %#v", hits)
	}
}

func TestLegacyJSONMigration(t *testing.T) {
	setLocalIndexTestHome(t)
	dir, err := store.EnsureBaseDir()
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacySnapshot{
		Version:  1,
		SyncedAt: time.Now().UTC(),
		Messages: []graph.Message{{ID: "legacy-1", Subject: "旧索引"}},
	}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "index.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	stats, err := LoadStats("")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Accounts) != 1 || stats.Accounts[0].Account != "legacy" || stats.TotalMessages != 1 {
		t.Fatalf("unexpected migrated stats: %#v", stats)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json.migrated")); err != nil {
		t.Fatal("legacy json was not renamed")
	}
}

func TestCompactAndResetAccount(t *testing.T) {
	setLocalIndexTestHome(t)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.replaceMessages("a@example.com", []graph.Message{{ID: "m1", Subject: "x"}}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := Compact(); err != nil {
		t.Fatal(err)
	}
	if err := ResetAccount("a@example.com"); err != nil {
		t.Fatal(err)
	}
	stats, err := LoadStats("")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalMessages != 0 {
		t.Fatalf("reset failed: %#v", stats)
	}
}

func BenchmarkSearch10K(b *testing.B) {
	home := b.TempDir()
	oldHome, oldUser := os.Getenv("HOME"), os.Getenv("USERPROFILE")
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	defer func() {
		os.Setenv("HOME", oldHome)
		os.Setenv("USERPROFILE", oldUser)
	}()
	s, err := Open()
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	messages := make([]graph.Message, 0, 10000)
	for i := 0; i < 10000; i++ {
		messages = append(messages, graph.Message{ID: "m", Subject: "账单通知", BodyPreview: "本月账单内容", ReceivedDateTime: time.Now()})
	}
	if err := s.replaceMessages("bench", messages); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Search("bench", "账单", "mail", 20); err != nil {
			b.Fatal(err)
		}
	}
}
