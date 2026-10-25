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
