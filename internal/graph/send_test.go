package graph

import "testing"

// TestBuildRecipients 验证邮箱地址会被正确转换，并拒绝明显非法的地址。
func TestBuildRecipients(t *testing.T) {
	recipients, err := buildRecipients([]string{" a@example.com ", "b@example.com", ""}, "收件人")
	if err != nil {
		t.Fatalf("合法邮箱不应返回错误: %v", err)
	}
	if len(recipients) != 2 {
		t.Fatalf("收件人数量 = %d, 期望 2", len(recipients))
	}

	if _, err := buildRecipients([]string{"not-an-email"}, "收件人"); err == nil {
		t.Fatal("非法邮箱应返回错误")
	}

	recipients, err = buildRecipients([]string{"", " "}, "收件人")
	if err != nil {
		t.Fatalf("空白邮箱不应返回错误: %v", err)
	}
	if len(recipients) != 0 {
		t.Fatalf("空白邮箱应被忽略，实际数量 = %d", len(recipients))
	}
}
