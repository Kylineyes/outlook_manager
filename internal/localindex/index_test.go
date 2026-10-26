package localindex

import (
	"testing"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func TestSaveLoadAndSearch(t *testing.T) {
	setLocalIndexTestHome(t)
	snapshot := &Snapshot{
		Messages: []graph.Message{{ID: "m1", Subject: "账单通知", BodyPreview: "本月账单", ReceivedDateTime: time.Now()}},
		Contacts: []graph.Contact{{ID: "c1", DisplayName: "张三", EmailAddresses: []graph.ContactEmailAddress{{Address: "a@example.com"}}}},
		Events:   []graph.CalendarEvent{{ID: "e1", Subject: "项目会议", Start: graph.DateTimeTimeZone{DateTime: "2026-11-01T09:00:00", TimeZone: "UTC"}}},
	}
	if err := Save(snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Contacts) != 1 || len(loaded.Events) != 1 {
		t.Fatalf("unexpected loaded: %#v", loaded)
	}
	if hits := Search(loaded, "账单", "mail", 10); len(hits) != 1 || hits[0].ID != "m1" {
		t.Fatalf("unexpected mail hits: %#v", hits)
	}
	if hits := Search(loaded, "项目", "event", 10); len(hits) != 1 || hits[0].ID != "e1" {
		t.Fatalf("unexpected event hits: %#v", hits)
	}
	if hits := Search(loaded, "a@example.com", "contact", 10); len(hits) != 1 || hits[0].ID != "c1" {
		t.Fatalf("unexpected contact hits: %#v", hits)
	}
}

func TestDeltaApplyHelpers(t *testing.T) {
	messages := upsertMessages([]graph.Message{{ID: "1", Subject: "old"}}, []graph.Message{{ID: "2", Subject: "new"}, {ID: "1", Subject: "updated"}})
	if len(messages) != 2 || messages[0].Subject != "updated" {
		t.Fatalf("unexpected upsert: %#v", messages)
	}
	messages = removeMessages(messages, []string{"1"})
	if len(messages) != 1 || messages[0].ID != "2" {
		t.Fatalf("unexpected remove: %#v", messages)
	}
}

func BenchmarkSearch(b *testing.B) {
	snapshot := &Snapshot{Messages: make([]graph.Message, 0, 1000)}
	for i := 0; i < 1000; i++ {
		snapshot.Messages = append(snapshot.Messages, graph.Message{ID: "m", Subject: "账单通知", BodyPreview: "本月账单内容"})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Search(snapshot, "账单", "mail", 20)
	}
}
