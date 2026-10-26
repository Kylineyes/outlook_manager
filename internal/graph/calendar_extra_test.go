package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type calendarExtraTransport struct {
	body string
}

func (t *calendarExtraTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		data, _ := io.ReadAll(req.Body)
		t.body = string(data)
	}
	payload := `{"value":[{"scheduleId":"a@example.com","availabilityView":"00","scheduleItems":[]}]}`
	if strings.Contains(req.URL.Path, "findMeetingTimes") {
		payload = `{"meetingTimeSuggestions":[{"confidence":0.9,"meetingTimeSlot":{"start":{"dateTime":"2026-11-01T09:00:00","timeZone":"UTC"},"end":{"dateTime":"2026-11-01T10:00:00","timeZone":"UTC"}}}]}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header), Request: req}, nil
}

func TestGetScheduleRequest(t *testing.T) {
	transport := &calendarExtraTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	result, err := GetSchedule(context.Background(), "token", []string{"a@example.com"}, start, start.Add(24*time.Hour), 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !strings.Contains(transport.body, `"availabilityViewInterval":30`) {
		t.Fatalf("unexpected schedule: %#v body=%s", result, transport.body)
	}
}

func TestFindMeetingTimesRequest(t *testing.T) {
	transport := &calendarExtraTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	result, err := FindMeetingTimes(context.Background(), "token", []string{"a@example.com"}, start, start.Add(24*time.Hour), 60, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !strings.Contains(transport.body, `"meetingDuration":"PT60M"`) {
		t.Fatalf("unexpected suggestions: %#v body=%s", result, transport.body)
	}
}

type deltaTransport struct{}

func (t *deltaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	payload := `{"value":[{"id":"m1","subject":"hello"}],"@odata.deltaLink":"https://graph.microsoft.com/v1.0/me/messages/delta?$deltatoken=1"}`
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header), Request: req}, nil
}

func TestDeltaMessages(t *testing.T) {
	oldClient := httpClient
	httpClient = &http.Client{Transport: &deltaTransport{}}
	defer func() { httpClient = oldClient }()
	result, err := DeltaMessages(context.Background(), "token", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "m1" || result.DeltaLink == "" {
		t.Fatalf("unexpected delta result: %#v", result)
	}
}

func TestResolveSeriesTarget(t *testing.T) {
	event := &CalendarEvent{ID: "occurrence", SeriesMasterID: "master", Type: "occurrence"}
	target, err := ResolveSeriesTarget(event, "single")
	if err != nil || target != "occurrence" {
		t.Fatalf("single target = %s err=%v", target, err)
	}
	target, err = ResolveSeriesTarget(event, "series")
	if err != nil || target != "master" {
		t.Fatalf("series target = %s err=%v", target, err)
	}
	if _, err := ResolveSeriesTarget(&CalendarEvent{ID: "single", Type: "singleInstance"}, "series"); err == nil {
		t.Fatal("expected non-recurring series error")
	}
}
