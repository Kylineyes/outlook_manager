package uploadstate

import (
	"testing"
)

func TestAttachmentAndSendState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	attachment := AttachmentState{Key: "a1", DraftID: "d1", Name: "a.bin", Path: "a.bin", Size: 100, Offset: 50, UploadURL: "https://upload"}
	if err := SaveAttachment(attachment); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAttachment("a1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Offset != 50 || loaded.UploadURL != "https://upload" {
		t.Fatalf("unexpected attachment state: %#v", loaded)
	}
	send := SendState{Key: "s1", DraftID: "d1", UploadsDone: true}
	if err := SaveSend(send); err != nil {
		t.Fatal(err)
	}
	loadedSend, err := LoadSend("s1")
	if err != nil {
		t.Fatal(err)
	}
	if loadedSend == nil || !loadedSend.UploadsDone {
		t.Fatalf("unexpected send state: %#v", loadedSend)
	}
	if err := DeleteAttachment("a1"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteSend("s1"); err != nil {
		t.Fatal(err)
	}
}
