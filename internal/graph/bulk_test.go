package graph

import (
	"context"
	"testing"
)

func TestBulkDeleteMessagesDryRunDeduplicates(t *testing.T) {
	result, err := BulkDeleteMessages(context.Background(), "token", []string{"a", "a", "b", " "}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.DryRun != true || result.Succeeded != 0 {
		t.Fatalf("unexpected bulk result: %#v", result)
	}
}

func TestBulkMoveRequiresFolder(t *testing.T) {
	if _, err := BulkMoveMessages(context.Background(), "token", []string{"a"}, "", false); err == nil {
		t.Fatal("expected error for empty folder")
	}
}

func TestBulkStatusDryRun(t *testing.T) {
	result, err := BulkSetMessagesRead(context.Background(), "token", []string{"a", "a", "b"}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || !result.DryRun || result.Succeeded != 0 {
		t.Fatalf("unexpected bulk read result: %#v", result)
	}
	flag, err := BulkSetMessagesFlag(context.Background(), "token", []string{"a"}, true, false)
	if err != nil || flag.Total != 1 || !flag.DryRun {
		t.Fatalf("unexpected bulk flag result: %#v %v", flag, err)
	}
	category, err := BulkAddMessagesCategory(context.Background(), "token", []string{"a"}, "重要", false)
	if err != nil || category.Total != 1 || !category.DryRun {
		t.Fatalf("unexpected bulk category result: %#v %v", category, err)
	}
}

func TestBulkStatusValidation(t *testing.T) {
	if _, err := BulkSetMessagesImportance(context.Background(), "token", []string{"a"}, "urgent", false); err == nil {
		t.Fatal("expected invalid importance error")
	}
	if _, err := BulkAddMessagesCategory(context.Background(), "token", []string{"a"}, " ", false); err == nil {
		t.Fatal("expected empty category error")
	}
}
