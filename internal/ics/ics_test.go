package ics

import (
	"bytes"
	"testing"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func TestICSRoundTrip(t *testing.T) {
	start := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	events := []graph.CalendarEvent{{
		ID:         "event-1",
		Subject:    "周会, 讨论; 计划",
		Start:      graph.DateTimeTimeZone{DateTime: start.Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		End:        graph.DateTimeTimeZone{DateTime: start.Add(time.Hour).Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		Location:   graph.EventLocation{DisplayName: "会议室 A"},
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
	parsed, err := Parse(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 {
		t.Fatalf("events = %d", len(parsed))
	}
	if parsed[0].Subject != "周会, 讨论; 计划" || parsed[0].RepeatType != "weekly" || parsed[0].RepeatCount != 5 {
		t.Fatalf("unexpected event: %#v", parsed[0])
	}
	if parsed[0].Start.Hour() != 9 || parsed[0].End.Hour() != 10 {
		t.Fatalf("unexpected times: %#v", parsed[0])
	}
}

func TestICSAllDay(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:1\r\nSUMMARY:全天\r\nDTSTART;VALUE=DATE:20261201\r\nDTEND;VALUE=DATE:20261202\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	events, err := Parse(bytes.NewBufferString(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !events[0].IsAllDay || events[0].Start.Day() != 1 || events[0].End.Day() != 2 {
		t.Fatalf("unexpected all-day: %#v", events)
	}
}
