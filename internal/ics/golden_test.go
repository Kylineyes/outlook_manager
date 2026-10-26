package ics

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

var updateGolden = flag.Bool("update", false, "更新 golden 文件")

func TestGoldenExport(t *testing.T) {
	oldNow := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = oldNow }()

	start := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	events := []graph.CalendarEvent{{
		ID:         "event-1",
		Subject:    "Golden 测试",
		Start:      graph.DateTimeTimeZone{DateTime: start.Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		End:        graph.DateTimeTimeZone{DateTime: start.Add(time.Hour).Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		Location:   graph.EventLocation{DisplayName: "A 会议室"},
		Body:       graph.ItemBody{Content: "第一行\n第二行"},
		Categories: []string{"工作"},
		Attendees:  []graph.EventAttendee{{EmailAddress: graph.EmailAddress{Address: "a@example.com"}}},
		Recurrence: &graph.Recurrence{
			Pattern: graph.RecurrencePattern{Type: "weekly", Interval: 1, DaysOfWeek: []string{"monday"}},
			Range:   graph.RecurrenceRange{Type: "numbered", StartDate: "2026-11-01", NumberOfOccurrences: 5},
		},
	}}
	var buffer bytes.Buffer
	if err := WriteEvents(&buffer, events); err != nil {
		t.Fatal(err)
	}
	goldenPath := filepath.Join("testdata", "calendar.golden.ics")
	if *updateGolden {
		if err := os.WriteFile(goldenPath, buffer.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buffer.Bytes(), expected) {
		t.Fatalf("golden mismatch:\n--- got ---\n%s\n--- want ---\n%s", buffer.String(), string(expected))
	}
}
