package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type eventUploadSessionTransport struct {
	path string
	body string
}

func (t *eventUploadSessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.path = req.URL.EscapedPath()
	data, _ := io.ReadAll(req.Body)
	t.body = string(data)
	return &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(strings.NewReader(`{"uploadUrl":"https://example.invalid/upload"}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestCreateEventAttachmentUploadSession(t *testing.T) {
	transport := &eventUploadSessionTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	uploadURL, err := createEventAttachmentUploadSession(context.Background(), "token", "event-id", MailAttachmentInput{
		Name: "large.bin",
		Size: 4 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("createEventAttachmentUploadSession returned error: %v", err)
	}
	if uploadURL != "https://example.invalid/upload" {
		t.Fatalf("uploadURL = %q", uploadURL)
	}
	if transport.path != "/v1.0/me/events/event-id/attachments/createUploadSession" {
		t.Fatalf("path = %s", transport.path)
	}
	if !strings.Contains(transport.body, `"attachmentType":"file"`) || !strings.Contains(transport.body, `"size":4194304`) {
		t.Fatalf("body = %s", transport.body)
	}
}
