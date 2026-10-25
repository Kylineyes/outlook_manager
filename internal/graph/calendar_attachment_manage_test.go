package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type calendarAttachmentManageTransport struct {
	methods []string
	paths   []string
}

func (t *calendarAttachmentManageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.methods = append(t.methods, req.Method)
	t.paths = append(t.paths, req.URL.EscapedPath())
	if req.Method == http.MethodDelete {
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
	}
	return &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(strings.NewReader(`{"id":"new-att","name":"new.txt","contentType":"text/plain","size":3}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestDeleteCalendarEventAttachment(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	transport := &calendarAttachmentManageTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	if err := DeleteCalendarEventAttachment(context.Background(), "token", "event-1", "att-1"); err != nil {
		t.Fatalf("DeleteCalendarEventAttachment returned error: %v", err)
	}
	if transport.methods[0] != http.MethodDelete || transport.paths[0] != "/v1.0/me/events/event-1/attachments/att-1" {
		t.Fatalf("unexpected request: %s %s", transport.methods[0], transport.paths[0])
	}
}

func TestReplaceCalendarEventAttachmentAddsThenDeletes(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	transport := &calendarAttachmentManageTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	attachment, err := ReplaceCalendarEventAttachment(context.Background(), "token", "event-1", "old-att", MailAttachmentInput{
		Name:         "new.txt",
		ContentType:  "text/plain",
		ContentBytes: []byte("new"),
	})
	if err != nil {
		t.Fatalf("ReplaceCalendarEventAttachment returned error: %v", err)
	}
	if attachment.ID != "new-att" {
		t.Fatalf("attachment ID = %q", attachment.ID)
	}
	if len(transport.methods) != 2 || transport.methods[0] != http.MethodPost || transport.methods[1] != http.MethodDelete {
		t.Fatalf("unexpected methods: %#v", transport.methods)
	}
	if transport.paths[1] != "/v1.0/me/events/event-1/attachments/old-att" {
		t.Fatalf("delete path = %s", transport.paths[1])
	}
}
