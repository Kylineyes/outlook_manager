package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckUpdateAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","html_url":"https://example.com/release","assets":[{"name":"a.zip","size":10}]}`))
	}))
	defer server.Close()
	old := APIBaseURL
	APIBaseURL = server.URL
	defer func() { APIBaseURL = old }()
	result, err := Check(context.Background(), "v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || result.Latest != "v0.2.0" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestCheckCurrentVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.1.0"}`))
	}))
	defer server.Close()
	old := APIBaseURL
	APIBaseURL = server.URL
	defer func() { APIBaseURL = old }()
	result, err := Check(context.Background(), "v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.UpdateAvailable {
		t.Fatalf("unexpected update: %#v", result)
	}
}

func TestCheckDevelopmentVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.2.0"}`))
	}))
	defer server.Close()
	old := APIBaseURL
	APIBaseURL = server.URL
	defer func() { APIBaseURL = old }()
	result, err := Check(context.Background(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsDevelopment || result.UpdateAvailable {
		t.Fatalf("unexpected development result: %#v", result)
	}
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"0.2.0", "0.1.0", 1},
		{"0.1.0", "0.2.0", -1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.0-beta.1", 1},
		{"1.0.0-beta.2", "1.0.0-beta.1", 1},
	}
	for _, tc := range cases {
		if got := compareVersion(tc.left, tc.right); got != tc.want {
			t.Fatalf("compareVersion(%q,%q) = %d, want %d", tc.left, tc.right, got, tc.want)
		}
	}
}
