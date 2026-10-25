package graph

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type folderTreeTransport struct{}

func (t *folderTreeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"value":[{"id":"root","displayName":"Inbox","childFolderCount":1}]}`
	if strings.Contains(req.URL.Path, "/root/childFolders") {
		body = `{"value":[{"id":"child","displayName":"Child","parentFolderId":"root"}]}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
}

func TestListMailFolderTree(t *testing.T) {
	oldClient := httpClient
	httpClient = &http.Client{Transport: &folderTreeTransport{}}
	defer func() { httpClient = oldClient }()

	tree, err := ListMailFolderTree(context.Background(), "token", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || len(tree[0].Children) != 1 || tree[0].Children[0].ID != "child" {
		t.Fatalf("unexpected tree: %#v", tree)
	}
}
