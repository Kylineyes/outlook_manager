package main

import (
	"reflect"
	"testing"
)

// TestParseMessageTargetArgs 验证邮件 ID、账号和 JSON 参数可以在任意顺序出现。
func TestParseMessageTargetArgs(t *testing.T) {
	target, err := parseMessageTargetArgs([]string{"msg-1", "--json", "--account", "a@example.com"}, true)
	if err != nil {
		t.Fatalf("解析参数失败: %v", err)
	}
	if target.MessageID != "msg-1" || target.Account != "a@example.com" || !target.JSON {
		t.Fatalf("解析结果不正确: %+v", target)
	}

	target, err = parseMessageTargetArgs([]string{"--account=a@example.com", "msg-2"}, false)
	if err != nil {
		t.Fatalf("解析参数失败: %v", err)
	}
	if target.MessageID != "msg-2" || target.Account != "a@example.com" || target.JSON {
		t.Fatalf("解析结果不正确: %+v", target)
	}

	if _, err := parseMessageTargetArgs([]string{"--json"}, true); err == nil {
		t.Fatal("缺少邮件 ID 应返回错误")
	}
}

// TestSplitAddresses 验证多个邮箱地址会按逗号拆分并清理空白。
func TestSplitAddresses(t *testing.T) {
	got := splitAddresses(" a@example.com, b@example.com ,, c@example.com ")
	want := []string{"a@example.com", "b@example.com", "c@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("拆分结果 = %#v, 期望 %#v", got, want)
	}
}
