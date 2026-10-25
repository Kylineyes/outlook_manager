package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestPostFormRetriesServiceUnavailable(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"ok"}`))
	}))
	defer server.Close()

	client := &oauthClient{httpClient: server.Client()}
	resp, body, err := client.postForm(context.Background(), server.URL, url.Values{"grant_type": {"refresh_token"}})
	if err != nil {
		t.Fatalf("postForm returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != `{"access_token":"ok"}` {
		t.Fatalf("body = %q", string(body))
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestPostFormDoesNotRetryBadRequest(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := &oauthClient{httpClient: server.Client()}
	resp, _, err := client.postForm(context.Background(), server.URL, url.Values{"grant_type": {"refresh_token"}})
	if err != nil {
		t.Fatalf("postForm returned error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}
