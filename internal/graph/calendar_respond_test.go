package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type calendarRespondTransport struct {
	method string
	path   string
	body   string
}

func (t *calendarRespondTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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

func TestRespondCalendarEventBuildsRequest(t *testing.T) {
	setGraphTestHome(t)
	transport := &calendarRespondTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	err := RespondCalendarEvent(context.Background(), "token", "event id", "tentative", "测试说明")
	if err != nil {
		t.Fatalf("RespondCalendarEvent returned error: %v", err)
	}
	if transport.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", transport.method)
	}
	if transport.path != "/v1.0/me/events/event%20id/tentativelyAccept" {
		t.Fatalf("path = %s", transport.path)
	}
	if !strings.Contains(transport.body, `"sendResponse":true`) {
		t.Fatalf("body missing sendResponse: %s", transport.body)
	}
	if !strings.Contains(transport.body, `"comment":"测试说明"`) {
		t.Fatalf("body missing comment: %s", transport.body)
	}
}

func TestRespondCalendarEventRejectsUnknownResponse(t *testing.T) {
	err := RespondCalendarEvent(context.Background(), "token", "event-id", "unknown", "")
	if err == nil {
		t.Fatal("expected error for unknown response")
	}
}
