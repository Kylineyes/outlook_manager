package store

import "testing"

// TestEnsureDefaultScopes 验证老配置会自动补齐新增默认 scope，且不会重复添加。
func TestEnsureDefaultScopes(t *testing.T) {
	cfg := Config{Scopes: []string{"User.Read"}}
	if !ensureDefaultScopes(&cfg) {
		t.Fatal("首次补齐默认 scope 应返回 true")
	}
	for _, scope := range defaultScopes {
		found := false
		for _, existing := range cfg.Scopes {
			if existing == scope {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("默认 scope 未被补齐: %s", scope)
		}
	}

	if ensureDefaultScopes(&cfg) {
		t.Fatal("第二次补齐不应再返回 true")
	}
}
