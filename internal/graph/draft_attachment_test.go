package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type draftAttachmentTransport struct {
	attempts int32
	methods  []string
	paths    []string
	bodies   []string
}

func (t *draftAttachmentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&t.attempts, 1)
	t.methods = append(t.methods, req.Method)
	t.paths = append(t.paths, req.URL.EscapedPath())
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	t.bodies = append(t.bodies, string(body))
	if req.Method == http.MethodDelete {
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
	}
	return &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(strings.NewReader(`{"id":"att-1","name":"test.txt","contentType":"text/plain","size":5}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestAddDraftAttachmentSmall(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	transport := &draftAttachmentTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	attachment, err := AddDraftAttachment(context.Background(), "token", "draft-1", MailAttachmentInput{
		Name:         "test.txt",
		ContentType:  "text/plain",
		ContentBytes: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("AddDraftAttachment returned error: %v", err)
	}
	if attachment.ID != "att-1" {
		t.Fatalf("attachment ID = %q", attachment.ID)
	}
	if transport.paths[0] != "/v1.0/me/messages/draft-1/attachments" {
		t.Fatalf("path = %s", transport.paths[0])
	}
	if !strings.Contains(transport.bodies[0], `"name":"test.txt"`) {
		t.Fatalf("body = %s", transport.bodies[0])
	}
}

func TestDeleteDraftAttachment(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	transport := &draftAttachmentTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	if err := DeleteDraftAttachment(context.Background(), "token", "draft-1", "att-1"); err != nil {
		t.Fatalf("DeleteDraftAttachment returned error: %v", err)
	}
	if transport.methods[0] != http.MethodDelete {
		t.Fatalf("method = %s", transport.methods[0])
	}
	if transport.paths[0] != "/v1.0/me/messages/draft-1/attachments/att-1" {
		t.Fatalf("path = %s", transport.paths[0])
	}
}

func TestReplaceDraftAttachmentAddsThenDeletes(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	transport := &draftAttachmentTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	attachment, err := ReplaceDraftAttachment(context.Background(), "token", "draft-1", "att-old", MailAttachmentInput{
		Name:         "new.txt",
		ContentType:  "text/plain",
		ContentBytes: []byte("new"),
	})
	if err != nil {
		t.Fatalf("ReplaceDraftAttachment returned error: %v", err)
	}
	if attachment.ID != "att-1" {
		t.Fatalf("attachment ID = %q", attachment.ID)
	}
	if len(transport.methods) != 2 || transport.methods[0] != http.MethodPost || transport.methods[1] != http.MethodDelete {
		t.Fatalf("unexpected methods: %#v", transport.methods)
	}
	if transport.paths[1] != "/v1.0/me/messages/draft-1/attachments/att-old" {
		t.Fatalf("delete path = %s", transport.paths[1])
	}
}
