package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type calendarForwardTransport struct {
	method string
	path   string
	body   string
}

func (t *calendarForwardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.method = req.Method
	t.path = req.URL.EscapedPath()
	data, _ := io.ReadAll(req.Body)
	t.body = string(data)
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestForwardCalendarEventBuildsRequest(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	transport := &calendarForwardTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	err := ForwardCalendarEvent(context.Background(), "token", "event id", "请参加", []string{"a@example.com", "b@example.com"})
	if err != nil {
		t.Fatalf("ForwardCalendarEvent returned error: %v", err)
	}
	if transport.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", transport.method)
	}
	if transport.path != "/v1.0/me/events/event%20id/forward" {
		t.Fatalf("path = %s", transport.path)
	}
	if !strings.Contains(transport.body, `"address":"a@example.com"`) || !strings.Contains(transport.body, `"address":"b@example.com"`) {
		t.Fatalf("body missing recipients: %s", transport.body)
	}
	if !strings.Contains(transport.body, `"comment":"请参加"`) {
		t.Fatalf("body missing comment: %s", transport.body)
	}
}
