package main

import "testing"

func TestToolAnnotations(t *testing.T) {
	readTool := newTool("mail_list", "列出邮件")
	if readTool.Annotations == nil || !readTool.Annotations.ReadOnlyHint || readTool.Annotations.IdempotentHint {
		// read-only 工具应当标记为只读，并且默认不应被误标为破坏性。
	}
	if readTool.Annotations == nil || !readTool.Annotations.ReadOnlyHint {
		t.Fatal("mail_list should be read-only")
	}
	if readTool.Annotations.DestructiveHint == nil || *readTool.Annotations.DestructiveHint {
		t.Fatal("mail_list should not be destructive")
	}
	deleteTool := newTool("mail_permanent_delete", "永久删除邮件")
	if deleteTool.Annotations == nil || deleteTool.Annotations.DestructiveHint == nil || !*deleteTool.Annotations.DestructiveHint {
		t.Fatal("mail_permanent_delete should be destructive")
	}
	localTool := newTool("account_list", "列出账号")
	if localTool.Annotations == nil || localTool.Annotations.OpenWorldHint == nil || *localTool.Annotations.OpenWorldHint {
		t.Fatal("account_list should not be open-world")
	}
}

func TestStructuredOutput(t *testing.T) {
	result, _, err := jsonResult(map[string]any{"value": 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.StructuredContent == nil {
		t.Fatal("structured content is nil")
	}
	tool := newTool("mail_list", "列出邮件")
	if tool.OutputSchema == nil {
		t.Fatal("output schema is nil")
	}
}
