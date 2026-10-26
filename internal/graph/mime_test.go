package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mimeTransport struct {
	path string
}

func (t *mimeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.path = req.URL.EscapedPath()
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("Subject: test\r\n\r\nbody")), Header: make(http.Header), Request: req}, nil
}

func TestGetMessageMIME(t *testing.T) {
	transport := &mimeTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()
	data, err := GetMessageMIME(context.Background(), "token", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Subject: test\r\n\r\nbody" || transport.path != "/v1.0/me/messages/abc/$value" {
		t.Fatalf("unexpected MIME export: %q %q", data, transport.path)
	}
}
