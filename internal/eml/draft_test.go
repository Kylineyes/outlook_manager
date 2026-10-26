package eml

import (
	"bytes"
	"testing"
)

func TestToDraftInput(t *testing.T) {
	message := &Message{
		Subject:     "测试",
		To:          []string{"to@example.com"},
		Cc:          []string{"cc@example.com"},
		HTMLBody:    "<p>正文</p>",
		Attachments: []Attachment{{Name: "a.txt", ContentType: "text/plain", Content: []byte("hello")}},
	}
	draft, err := ToDraftInput(message)
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Cleanup()
	if draft.Input.ContentType != "HTML" || draft.Input.Body != "<p>正文</p>" {
		t.Fatalf("unexpected body: %#v", draft.Input)
	}
	if len(draft.Input.Attachments) != 1 || string(draft.Input.Attachments[0].ContentBytes) != "hello" {
		t.Fatalf("unexpected attachments: %#v", draft.Input.Attachments)
	}
	if _, err := ToDraftInput(&Message{Subject: "no recipient"}); err == nil {
		t.Fatal("expected missing recipient error")
	}
}

func TestToDraftInputLargeAttachmentUsesTempFile(t *testing.T) {
	content := bytes.Repeat([]byte("x"), simpleAttachmentLimit+1)
	draft, err := ToDraftInput(&Message{To: []string{"to@example.com"}, TextBody: "body", Attachments: []Attachment{{Name: "big.bin", Content: content}}})
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Cleanup()
	if len(draft.Input.Attachments) != 1 || draft.Input.Attachments[0].Path == "" || draft.Input.Attachments[0].ContentBytes != nil {
		t.Fatalf("large attachment not using temp file: %#v", draft.Input.Attachments)
	}
}
