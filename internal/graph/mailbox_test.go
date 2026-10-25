package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mailboxTransport struct {
	method string
	path   string
	body   string
}

func (t *mailboxTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.method = req.Method
	t.path = req.URL.EscapedPath()
	data := make([]byte, 0)
	if req.Body != nil {
		data, _ = io.ReadAll(req.Body)
	}
	t.body = string(data)
	payload := `{"timeZone":"UTC","language":{"locale":"zh-CN"}}`
	if req.Method == http.MethodPatch {
		payload = `{"timeZone":"Asia/Shanghai","automaticRepliesSetting":{"status":"alwaysEnabled"}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header), Request: req}, nil
}

func TestMailboxSettingsRequests(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	transport := &mailboxTransport{}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport}
	defer func() { httpClient = oldClient }()

	settings, err := GetMailboxSettings(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if settings.TimeZone != "UTC" || transport.path != "/v1.0/me/mailboxSettings" {
		t.Fatalf("unexpected get: %#v %#v", settings, transport)
	}
	updated, err := SetMailboxTimeZone(context.Background(), "token", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if updated.TimeZone != "Asia/Shanghai" || transport.method != http.MethodPatch || !strings.Contains(transport.body, `"timeZone":"Asia/Shanghai"`) {
		t.Fatalf("unexpected timezone update: %#v %#v", updated, transport)
	}
	replyUpdated, err := SetAutomaticReplies(context.Background(), "token", AutomaticRepliesSetting{Status: "alwaysEnabled", InternalReplyMessage: "内部回复"})
	if err != nil {
		t.Fatal(err)
	}
	if replyUpdated.AutomaticRepliesSetting.Status != "alwaysEnabled" || !strings.Contains(transport.body, `"internalReplyMessage":"内部回复"`) {
		t.Fatalf("unexpected auto reply update: %#v %#v", replyUpdated, transport)
	}
}
