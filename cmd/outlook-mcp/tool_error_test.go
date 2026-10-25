package main

import (
	"errors"
	"testing"
)

func TestToolErrorResult(t *testing.T) {
	result := toolErrorResult(errors.New("Graph 返回 404 not found"))
	if !result.IsError {
		t.Fatal("expected IsError true")
	}
	payload, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("unexpected structured content: %#v", result.StructuredContent)
	}
	if payload["success"] != false {
		t.Fatal("expected success false")
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "not_found" {
		t.Fatalf("unexpected error: %#v", payload["error"])
	}
}

func TestToolSchemaIsToolSpecific(t *testing.T) {
	tool := newTool("mail_read", "读取邮件")
	schema, ok := tool.OutputSchema.(map[string]any)
	if !ok {
		t.Fatalf("unexpected schema: %#v", tool.OutputSchema)
	}
	if schema["$id"] != "https://outlook-manager.local/schemas/tools/mail_read.json" {
		t.Fatalf("unexpected schema id: %#v", schema["$id"])
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil || props["result"] == nil || props["error"] == nil {
		t.Fatalf("schema missing result/error: %#v", schema)
	}
}
