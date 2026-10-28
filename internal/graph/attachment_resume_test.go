package graph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Kylineyes/outlook-manager/internal/uploadstate"
)

func TestUploadAttachmentResumableFromOffset(t *testing.T) {
	setGraphTestHome(t)
	size := int64(uploadChunkSize + 100)
	file, err := os.CreateTemp(t.TempDir(), "resume-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(make([]byte, size)); err != nil {
		t.Fatal(err)
	}
	file.Close()
	attachment := MailAttachmentInput{Name: "resume.bin", Path: file.Name(), Size: size}
	key := attachmentUploadKey("draft1", attachment)
	var mu sync.Mutex
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/attachments/createUploadSession"):
			_, _ = fmt.Fprintf(w, `{"uploadUrl":"%s/upload"}`, "http://"+r.Host)
		case r.URL.Path == "/upload" && r.Method == http.MethodGet:
			_, _ = fmt.Fprintf(w, `{"nextExpectedRanges":["%d-"]}`, uploadChunkSize)
		case r.URL.Path == "/upload" && r.Method == http.MethodPut:
			mu.Lock()
			ranges = append(ranges, r.Header.Get("Content-Range"))
			mu.Unlock()
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	oldBase, oldClient := graphBaseURL, httpClient
	graphBaseURL = server.URL + "/v1.0"
	httpClient = server.Client()
	defer func() { graphBaseURL, httpClient = oldBase, oldClient }()

	if err := uploadstate.SaveAttachment(uploadstate.AttachmentState{Key: key, DraftID: "draft1", Name: attachment.Name, Path: attachment.Path, Size: attachment.Size, Offset: uploadChunkSize, UploadURL: server.URL + "/upload"}); err != nil {
		t.Fatal(err)
	}
	var progress []UploadProgress
	ctx := WithUploadProgress(context.Background(), func(value UploadProgress) { progress = append(progress, value) })
	if err := uploadAttachmentResumable(ctx, "token", "draft1", attachment); err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 1 || !strings.HasPrefix(ranges[0], fmt.Sprintf("bytes %d-", uploadChunkSize)) {
		t.Fatalf("ranges = %#v", ranges)
	}
	if len(progress) == 0 || !progress[len(progress)-1].Done || progress[len(progress)-1].Uploaded != size {
		t.Fatalf("progress = %#v", progress)
	}
	state, err := uploadstate.LoadAttachment(key)
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatalf("state should be deleted: %#v", state)
	}
}
