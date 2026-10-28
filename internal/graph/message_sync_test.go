package graph

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type messageSyncTransport struct{ query url.Values }

func (t *messageSyncTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.query = req.URL.Query()
	payload := `{"value":[{"id":"m1","subject":"changed","lastModifiedDateTime":"2026-11-01T09:00:00Z"}]}`
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header), Request: req}, nil
}

func TestListMessagesModifiedSince(t *testing.T) {
	transport := &messageSyncTransport{}
	old := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = old }()
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	messages, err := ListMessagesModifiedSince(context.Background(), "token", since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != "m1" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if !strings.Contains(transport.query.Get("$filter"), "lastModifiedDateTime ge") {
		t.Fatalf("unexpected filter: %s", transport.query.Get("$filter"))
	}
}
