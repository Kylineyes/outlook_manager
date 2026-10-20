package graph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

func TestUploadAttachmentToSessionUploadsChunks(t *testing.T) {
	size := int64(uploadChunkSize + 100)
	file, err := os.CreateTemp(t.TempDir(), "large-attachment-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(make([]byte, size)); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var ranges []string
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Content-Range"))
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		attempt := len(ranges)
		mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		if attempt == 1 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	err = uploadAttachmentToSession(context.Background(), server.URL, MailAttachmentInput{
		Name: "large.bin",
		Path: file.Name(),
		Size: size,
	})
	if err != nil {
		t.Fatalf("uploadAttachmentToSession returned error: %v", err)
	}
	if len(ranges) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(ranges))
	}
	wantFirst := fmt.Sprintf("bytes 0-%d/%d", uploadChunkSize-1, size)
	wantSecond := fmt.Sprintf("bytes %d-%d/%d", uploadChunkSize, size-1, size)
	if ranges[0] != wantFirst {
		t.Fatalf("first range = %q, want %q", ranges[0], wantFirst)
	}
	if ranges[1] != wantSecond {
		t.Fatalf("second range = %q, want %q", ranges[1], wantSecond)
	}
	for _, auth := range authHeaders {
		if auth != "" {
			t.Fatalf("upload session request must not set Authorization header")
		}
	}
}

func TestHasLargeAttachments(t *testing.T) {
	if hasLargeAttachments([]MailAttachmentInput{{Size: simpleAttachmentLimit}}) {
		t.Fatal("simple attachment should not be treated as large")
	}
	if !hasLargeAttachments([]MailAttachmentInput{{Path: "large.bin", Size: simpleAttachmentLimit + 1}}) {
		t.Fatal("attachment over simple limit should be treated as large")
	}
	if !hasLargeAttachments([]MailAttachmentInput{{Size: 2 * 1024 * 1024}, {Size: 2 * 1024 * 1024}}) {
		t.Fatal("attachments whose total exceeds simple limit should be treated as large")
	}
}
