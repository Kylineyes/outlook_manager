package localindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
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

func TestHighWaterPersistence(t *testing.T) {
	setLocalIndexTestHome(t)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	meta, err := s.loadAccountMeta("a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	water := time.Now().UTC().Truncate(time.Second)
	meta.MessageHighWater = water
	if err := s.saveAccountMeta(meta); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	loaded, err := s.loadAccountMeta("a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.MessageHighWater.Equal(water) {
		t.Fatalf("high water = %s, want %s", loaded.MessageHighWater, water)
	}
}

func TestPruneMessages(t *testing.T) {
	setLocalIndexTestHome(t)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -120)
	recent := time.Now().UTC()
	if err := s.replaceMessages("a@example.com", []graph.Message{
		{ID: "old", Subject: "old mail", ReceivedDateTime: old},
		{ID: "new", Subject: "new mail", ReceivedDateTime: recent},
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	result, err := PruneMessages("a@example.com", time.Now().UTC().AddDate(0, 0, -90))
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", result.Deleted)
	}
	stats, err := LoadStats("a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalMessages != 1 {
		t.Fatalf("messages after prune = %d", stats.TotalMessages)
	}
}

func benchmarkSearch(b *testing.B, count int) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()
	seedBenchmarkMessages(b, s, "bench", count)
	b.ResetTimer()
	reportBenchmarkMetrics(b, s)
	for i := 0; i < b.N; i++ {
		if _, err := s.Search("bench", "账单", "mail", 20); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearch10K(b *testing.B)  { benchmarkSearch(b, 10000) }
func BenchmarkSearch100K(b *testing.B) { benchmarkSearch(b, 100000) }
func BenchmarkSearch500K(b *testing.B) { benchmarkSearch(b, 500000) }

func BenchmarkMultiAccountSearch(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()
	accounts := []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com"}
	for _, account := range accounts {
		seedBenchmarkMessages(b, s, account, 25000)
	}
	var counter uint64
	b.ResetTimer()
	reportBenchmarkMetrics(b, s)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			index := atomic.AddUint64(&counter, 1) % uint64(len(accounts))
			if _, err := s.Search(accounts[index], "账单", "mail", 20); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkBulkUpsert10K(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()
	messages := benchmarkMessages("bench", 0, 10000)
	b.ResetTimer()
	b.ReportMetric(10000, "rows")
	for i := 0; i < b.N; i++ {
		if err := s.applyMessageDelta("bench", messages, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBulkUpsert100K(b *testing.B) {
	s, cleanup := setupBenchmarkStore(b)
	defer cleanup()
	messages := benchmarkMessages("bench", 0, 100000)
	b.ResetTimer()
	b.ReportMetric(100000, "rows")
	for i := 0; i < b.N; i++ {
		if err := s.applyMessageDelta("bench", messages, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func setupBenchmarkStore(b *testing.B) (*Store, func()) {
	b.Helper()
	home := b.TempDir()
	oldHome, oldUser := os.Getenv("HOME"), os.Getenv("USERPROFILE")
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	s, err := Open()
	if err != nil {
		b.Fatal(err)
	}
	return s, func() {
		_ = s.Close()
		os.Setenv("HOME", oldHome)
		os.Setenv("USERPROFILE", oldUser)
	}
}

func seedBenchmarkMessages(b *testing.B, s *Store, account string, count int) {
	b.Helper()
	const batchSize = 5000
	for start := 0; start < count; start += batchSize {
		end := start + batchSize
		if end > count {
			end = count
		}
		messages := benchmarkMessages(account, start, end-start)
		if err := s.applyMessageDelta(account, messages, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkMessages(account string, start, count int) []graph.Message {
	messages := make([]graph.Message, 0, count)
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		messages = append(messages, graph.Message{
			ID:               fmt.Sprintf("%s-%d", account, start+i),
			Subject:          "账单通知",
			BodyPreview:      "本月账单内容和对账提醒",
			ReceivedDateTime: now.Add(-time.Duration(i) * time.Minute),
			From:             graph.MessageFrom{EmailAddress: graph.EmailAddress{Address: "billing@example.com"}},
		})
	}
	return messages
}

func reportBenchmarkMetrics(b *testing.B, s *Store) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	b.ReportMetric(float64(memory.Alloc), "alloc_bytes")
	var size int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(s.path + suffix); err == nil {
			size += info.Size()
		}
	}
	b.ReportMetric(float64(size), "db_bytes")
}

func TestSearchDocsMigration(t *testing.T) {
	setLocalIndexTestHome(t)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.replaceMessages("a@example.com", []graph.Message{{ID: "m1", Subject: "迁移测试", ReceivedDateTime: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM search_docs`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM meta WHERE key = 'search_docs_migrated'`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM search_docs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("search_docs = %d, want 1", count)
	}
	hits, err := s.Search("a@example.com", "迁移", "mail", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
}
