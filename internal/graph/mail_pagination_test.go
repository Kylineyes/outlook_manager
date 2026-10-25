package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type mailPaginationTransport struct {
	attempts int32
}

func (t *mailPaginationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempt := atomic.AddInt32(&t.attempts, 1)
	body := `{"value":[{"id":"2","subject":"two"}]}`
	if attempt == 1 {
		body = `{"value":[{"id":"1","subject":"one"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages?page=2"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestListMessagesInFolderAllFollowsNextLink(t *testing.T) {
	transport := &mailPaginationTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	messages, err := ListMessagesInFolderAll(context.Background(), "token", "inbox", 10)
	if err != nil {
		t.Fatalf("ListMessagesInFolderAll returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if messages[0].ID != "1" || messages[1].ID != "2" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if got := atomic.LoadInt32(&transport.attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}
