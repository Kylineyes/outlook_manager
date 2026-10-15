package auth

import (
	"encoding/base64"
	"testing"
	"time"
)

// TestParseIDTokenClaims 验证 id_token payload 能被正确解析。
func TestParseIDTokenClaims(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"preferred_username":"a@example.com","oid":"oid-1","tid":"tid-1","name":"测试用户"}`))
	token := "header." + payload + ".signature"

	claims, err := parseIDTokenClaims(token)
	if err != nil {
		t.Fatalf("解析 id_token 失败: %v", err)
	}
	if claims.PreferredUsername != "a@example.com" {
		t.Fatalf("preferred_username = %q", claims.PreferredUsername)
	}
	if claims.OID != "oid-1" || claims.TID != "tid-1" {
		t.Fatalf("oid/tid 解析结果不正确: %+v", claims)
	}
	if claims.Name != "测试用户" {
		t.Fatalf("name = %q", claims.Name)
	}
}

// TestBuildHomeAccountID 验证本地账号标识的生成优先级。
func TestBuildHomeAccountID(t *testing.T) {
	if got := buildHomeAccountID(idTokenClaims{OID: "oid", TID: "tid"}, "a@example.com"); got != "oid.tid" {
		t.Fatalf("oid/tid 标识 = %q", got)
	}
	if got := buildHomeAccountID(idTokenClaims{Subject: "sub", TID: "tid"}, "a@example.com"); got != "sub.tid" {
		t.Fatalf("subject/tid 标识 = %q", got)
	}
	if got := buildHomeAccountID(idTokenClaims{}, "A@Example.com"); got != "a@example.com" {
		t.Fatalf("用户名字标识 = %q", got)
	}
}

// TestSplitScopes 验证空格分隔的 scope 字符串会被正确拆分。
func TestSplitScopes(t *testing.T) {
	scopes := splitScopes("User.Read   Mail.Read offline_access")
	if len(scopes) != 3 {
		t.Fatalf("scope 数量 = %d, 期望 3", len(scopes))
	}
}

// TestTokenExpiry 验证 expires_in 会被转换为合理的过期时间。
func TestTokenExpiry(t *testing.T) {
	now := time.Now().UTC()
	expiry := tokenExpiry(3600)
	if expiry.Before(now.Add(59*time.Minute)) || expiry.After(now.Add(61*time.Minute)) {
		t.Fatalf("过期时间不在预期范围内: %s", expiry)
	}
}
