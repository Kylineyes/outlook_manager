package graph

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type queryTransport struct {
	lastURL *url.URL
}

func (t *queryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.lastURL = req.URL
	messages := []Message{{ID: "1", Subject: "Invoice", From: MessageFrom{EmailAddress: EmailAddress{Address: "billing@example.com"}}}}
	data, _ := json.Marshal(map[string]any{"value": messages})
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(data))), Header: make(http.Header), Request: req}, nil
}

func TestQueryMessagesUsesFilterWithoutOrderby(t *testing.T) {
	transport := &queryTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	messages, err := QueryMessages(context.Background(), "token", MessageQuery{Folder: "inbox", From: "billing@example.com", Top: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != "1" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	filter := transport.lastURL.Query().Get("$filter")
	if !strings.Contains(filter, "from/emailAddress/address eq 'billing@example.com'") {
		t.Fatalf("filter = %s", filter)
	}
	if transport.lastURL.Query().Get("$orderby") != "" {
		t.Fatalf("orderby should be omitted when filter exists: %s", transport.lastURL.Query().Get("$orderby"))
	}
}

func TestQueryMessagesAddsOrderbyWithoutFilter(t *testing.T) {
	transport := &queryTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	if _, err := QueryMessages(context.Background(), "token", MessageQuery{Folder: "inbox", Top: 10}); err != nil {
		t.Fatal(err)
	}
	if transport.lastURL.Query().Get("$orderby") != "receivedDateTime desc" {
		t.Fatalf("orderby = %s", transport.lastURL.Query().Get("$orderby"))
	}
}
