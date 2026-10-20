package graph

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDoGraphRequestRetriesServiceUnavailable(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := doGraphRequest(req)
	if err != nil {
		t.Fatalf("doGraphRequest returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestDoGraphRequestDoesNotRetryBadRequest(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := doGraphRequest(req)
	if err != nil {
		t.Fatalf("doGraphRequest returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

type retryTransport struct {
	attempts int32
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if atomic.AddInt32(&t.attempts, 1) == 1 {
		return nil, errors.New("connection reset by peer")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestDoGraphRequestRetriesNetworkError(t *testing.T) {
	transport := &retryTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.invalid/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := doGraphRequest(req)
	if err != nil {
		t.Fatalf("doGraphRequest returned error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&transport.attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}
