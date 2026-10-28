package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartupCheckCachesResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://example.com/v0.2.0"}`))
	}))
	defer server.Close()
	old := APIBaseURL
	APIBaseURL = server.URL
	defer func() { APIBaseURL = old }()

	result, err := StartupCheck(context.Background(), "v0.1.0", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || result.Latest != "v0.2.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("calls = %d", calls)
	}
	cached, err := StartupCheck(context.Background(), "v0.1.0", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !cached.UpdateAvailable || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("cache miss: %#v calls=%d", cached, calls)
	}
	if _, err := os.Stat(mustCheckCachePath(t)); err != nil {
		t.Fatal(err)
	}
	current, err := StartupCheck(context.Background(), "v0.2.0", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if current.UpdateAvailable {
		t.Fatalf("same version should not update: %#v", current)
	}
}

func mustCheckCachePath(t *testing.T) string {
	t.Helper()
	path, err := CheckCachePath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}
