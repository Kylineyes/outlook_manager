package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type multiFolderRuleTransport struct{}

func (t *multiFolderRuleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"value":[{"id":"m1","subject":"needle this"}]}`
	if strings.Contains(req.URL.Path, "folder2") {
		body = `{"value":[{"id":"m2","subject":"other"}]}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestApplyMessageRuleToFoldersDryRun(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	oldClient := httpClient
	httpClient = &http.Client{Transport: &multiFolderRuleTransport{}}
	defer func() { httpClient = oldClient }()

	rule := &MessageRule{
		ID:          "rule-1",
		DisplayName: "多文件夹规则",
		Conditions:  map[string]any{"subjectContains": []any{"needle"}},
		Actions:     map[string]any{"markAsRead": true},
	}
	result, err := ApplyMessageRuleToFolders(context.Background(), "token", rule, []string{"folder1", "folder2"}, 10, false)
	if err != nil {
		t.Fatalf("ApplyMessageRuleToFolders returned error: %v", err)
	}
	if result.Scanned != 2 || result.Matched != 1 || result.Applied != 0 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if len(result.FolderIDs) != 2 || result.FolderIDs[0] != "folder1" || result.FolderIDs[1] != "folder2" {
		t.Fatalf("unexpected folder ids: %#v", result.FolderIDs)
	}
	if len(result.Matches) != 1 || result.Matches[0].FolderID != "folder1" {
		t.Fatalf("unexpected matches: %#v", result.Matches)
	}
}
