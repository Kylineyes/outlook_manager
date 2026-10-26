package store

import (
	"testing"
)

func TestMailViewsRoundTrip(t *testing.T) {
	setStoreTestHome(t)
	view := MailView{Name: "未读", Folder: "inbox", UnreadOnly: true, Top: 20}
	if _, err := UpsertMailView(view); err != nil {
		t.Fatal(err)
	}
	file, err := LoadMailViews()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Views) != 1 || file.Views[0].Name != "未读" || !file.Views[0].UnreadOnly {
		t.Fatalf("unexpected views: %#v", file.Views)
	}
	view.Top = 50
	if _, err := UpsertMailView(view); err != nil {
		t.Fatal(err)
	}
	file, err = LoadMailViews()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Views) != 1 || file.Views[0].Top != 50 {
		t.Fatalf("upsert failed: %#v", file.Views)
	}
	if _, err := DeleteMailView("未读"); err != nil {
		t.Fatal(err)
	}
	file, err = LoadMailViews()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Views) != 0 {
		t.Fatalf("delete failed: %#v", file.Views)
	}
}
