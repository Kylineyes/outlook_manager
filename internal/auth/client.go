// Package auth 封装 Outlook Manager 的登录与 token 持久化。
//
// 当前实现直接调用 Microsoft 身份平台的 OAuth2 Device Code Flow，
// 不使用 MSAL Go，原因是 MSAL Go 对个人 Microsoft 账号的 offline_access
// 存在已知误判，会把正常的 token 响应当成 declined scope。
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// Client 是认证模块对外暴露的客户端。
type Client struct {
	config store.Config
	oauth  *oauthClient
}

// idTokenClaims 是从 id_token 中解析出的最小账号信息。
type idTokenClaims struct {
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	OID               string `json:"oid"`
	TID               string `json:"tid"`
	Name              string `json:"name"`
	Subject           string `json:"sub"`
}

// NewClient 创建认证客户端。
// 创建过程中会确保本地数据目录和默认 config.json 存在，不会触发网络请求。
func NewClient() (*Client, error) {
	config, err := store.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("加载认证配置失败: %w", err)
	}
	return &Client{config: config, oauth: newOAuthClient(config)}, nil
}

// Config 返回当前使用的认证配置副本。
func (c *Client) Config() store.Config {
	cfg := c.config
	cfg.Scopes = append([]string(nil), c.config.Scopes...)
	return cfg
}

// LoginDeviceCode 执行完整的 Device Code 登录流程。
//
// 流程：
//  1. 请求设备代码并打印给用户；
//  2. 轮询 token 接口，等待用户在浏览器完成授权；
//  3. 解析 id_token 获取账号元数据；
//  4. 把 token 加密写入 token-cache.bin。
func (c *Client) LoginDeviceCode(ctx context.Context, out io.Writer) (*LoginResult, error) {
	deviceCode, err := c.oauth.requestDeviceCode(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(out, deviceCode.Message)
	fmt.Fprintf(out, "设备代码: %s\n", deviceCode.UserCode)
	fmt.Fprintf(out, "验证地址: %s\n", deviceCode.VerificationURI)

	token, err := c.oauth.pollDeviceCodeToken(ctx, deviceCode.DeviceCode, deviceCode.Interval, deviceCode.ExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("等待 Device Code 授权失败: %w", err)
	}

	claims, _ := parseIDTokenClaims(token.IDToken)
	account := accountFromToken(token, claims, c.config.Scopes)
	cached := cachedAccountFromToken(account, token)

	cache, err := loadTokenCache()
	if err != nil {
		return nil, err
	}
	upsertCachedAccount(cache, cached)
	if err := saveTokenCache(cache); err != nil {
		return nil, fmt.Errorf("保存 token 缓存失败: %w", err)
	}

	return &LoginResult{Account: account, AccessToken: token.AccessToken}, nil
}

// Accounts 读取本地加密 token 缓存中的账号列表。
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cache, err := loadTokenCache()
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, 0, len(cache.Accounts))
	for _, item := range cache.Accounts {
		accounts = append(accounts, item.toAccount())
	}
	return accounts, nil
}

// AcquireTokenSilent 使用本地 refresh token 静默刷新 access token。
// 成功后会把新的 token 写回加密缓存。
func (c *Client) AcquireTokenSilent(ctx context.Context, account Account) (*LoginResult, error) {
	cache, err := loadTokenCache()
	if err != nil {
		return nil, err
	}
	index := findCachedAccount(cache, account)
	if index < 0 {
		return nil, fmt.Errorf("本地找不到账号 %s，请重新登录", account.Username)
	}

	cached := cache.Accounts[index]
	if cached.RefreshToken == "" {
		return nil, fmt.Errorf("账号 %s 没有 refresh token，请重新登录", account.Username)
	}

	token, err := c.oauth.refreshAccessToken(ctx, cached.RefreshToken)
	if err != nil {
		return nil, err
	}

	if token.RefreshToken == "" {
		// 服务端没有返回新的 refresh token 时，继续沿用旧值。
		token.RefreshToken = cached.RefreshToken
	}
	if token.IDToken == "" {
		token.IDToken = cached.IDToken
	}

	claims, _ := parseIDTokenClaims(token.IDToken)
	account = accountFromToken(token, claims, cached.Scopes)
	cached = cachedAccountFromToken(account, token)

	cache.Accounts[index] = cached
	if err := saveTokenCache(cache); err != nil {
		return nil, fmt.Errorf("更新 token 缓存失败: %w", err)
	}
	return &LoginResult{Account: account, AccessToken: token.AccessToken}, nil
}

// accountFromToken 根据 token 和 id_token 声明组装账号元数据。
func accountFromToken(token *tokenResponse, claims idTokenClaims, fallbackScopes []string) Account {
	username := firstNonEmpty(claims.PreferredUsername, claims.Email)
	scopes := splitScopes(token.Scope)
	if len(scopes) == 0 {
		scopes = append([]string(nil), fallbackScopes...)
	}
	return Account{
		Username:           username,
		HomeAccountID:      buildHomeAccountID(claims, username),
		Environment:        "login.windows.net",
		TenantID:           claims.TID,
		DisplayName:        claims.Name,
		Scopes:             scopes,
		ExpiresAt:          tokenExpiry(token.ExpiresIn),
		RefreshTokenCached: token.RefreshToken != "",
	}
}

// cachedAccountFromToken 把账号元数据与 token 组合成加密缓存结构。
func cachedAccountFromToken(account Account, token *tokenResponse) cachedAccount {
	return cachedAccount{
		Username:      account.Username,
		HomeAccountID: account.HomeAccountID,
		Environment:   account.Environment,
		TenantID:      account.TenantID,
		DisplayName:   account.DisplayName,
		Scopes:        append([]string(nil), account.Scopes...),
		AccessToken:   token.AccessToken,
		RefreshToken:  token.RefreshToken,
		IDToken:       token.IDToken,
		ExpiresAt:     account.ExpiresAt,
	}
}

// parseIDTokenClaims 解析 JWT 格式的 id_token payload。
// 解析失败不会阻止登录，因为后续还可以通过 Graph /me 获取账号信息。
func parseIDTokenClaims(idToken string) (idTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return idTokenClaims{}, fmt.Errorf("id_token 格式不正确")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return idTokenClaims{}, fmt.Errorf("解码 id_token payload 失败: %w", err)
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return idTokenClaims{}, fmt.Errorf("解析 id_token claims 失败: %w", err)
	}
	return claims, nil
}

// buildHomeAccountID 生成稳定的本地账号标识。
// 优先使用 oid.tid；缺失时退回用户名，保证同一账号仍能匹配到同一档案。
func buildHomeAccountID(claims idTokenClaims, username string) string {
	if claims.OID != "" && claims.TID != "" {
		return claims.OID + "." + claims.TID
	}
	if claims.Subject != "" && claims.TID != "" {
		return claims.Subject + "." + claims.TID
	}
	if username != "" {
		return strings.ToLower(username)
	}
	return ""
}

// splitScopes 把 OAuth2 响应中的空格分隔 scope 转换为切片。
func splitScopes(scope string) []string {
	fields := strings.Fields(scope)
	return append([]string(nil), fields...)
}

// tokenExpiry 根据 expires_in 计算 access token 的过期时间。
func tokenExpiry(expiresIn int) time.Time {
	if expiresIn <= 0 {
		return time.Now().UTC()
	}
	return time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
