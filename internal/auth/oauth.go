package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// deviceCodeGrantType 是 OAuth2 设备代码流程换取 token 时使用的 grant_type。
const deviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// oauthClient 是直接调用 Microsoft 身份平台的轻量 OAuth2 客户端。
//
// 这里没有使用 MSAL Go，是因为当前 MSAL Go 对 Microsoft 个人账号的
// offline_access 处理存在已知问题：token 响应中的 scope 通常不会返回
// offline_access，但 MSAL Go 会把它判定为 declined scope 并返回错误。
// 该问题见 microsoft-authentication-library-for-go issue #416。
type oauthClient struct {
	httpClient *http.Client
	clientID   string
	authority  string
	scopes     []string
}

// deviceCodeResponse 是设备代码接口的响应。
type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	Message                 string `json:"message"`
}

// tokenResponse 是 token 接口的响应。
// refresh_token 可能为空，表示服务端没有返回新的 refresh token。
type tokenResponse struct {
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	ExpiresIn        int    `json:"expires_in"`
	ExtExpiresIn     int    `json:"ext_expires_in"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// newOAuthClient 根据本地配置创建 OAuth2 客户端。
func newOAuthClient(config store.Config) *oauthClient {
	return &oauthClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		clientID:   config.ClientID,
		authority:  strings.TrimRight(config.Authority, "/"),
		scopes:     append([]string(nil), config.Scopes...),
	}
}

// deviceCodeEndpoint 返回设备代码接口地址。
func (c *oauthClient) deviceCodeEndpoint() string {
	return c.authority + "/oauth2/v2.0/devicecode"
}

// tokenEndpoint 返回 token 接口地址。
func (c *oauthClient) tokenEndpoint() string {
	return c.authority + "/oauth2/v2.0/token"
}

// requestScopes 返回实际发送给身份平台的 scope 列表。
// 除了业务 scope，还确保包含 openid、profile、offline_access：
//   - openid / profile：获取 id token 和基础账号信息；
//   - offline_access：要求返回 refresh token，便于本地静默刷新。
func (c *oauthClient) requestScopes() []string {
	scopes := append([]string(nil), c.scopes...)
	for _, required := range []string{"openid", "profile", "offline_access"} {
		found := false
		for _, scope := range scopes {
			if strings.EqualFold(scope, required) {
				found = true
				break
			}
		}
		if !found {
			scopes = append(scopes, required)
		}
	}
	return scopes
}

// requestDeviceCode 请求设备代码。
func (c *oauthClient) requestDeviceCode(ctx context.Context) (*deviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("scope", strings.Join(c.requestScopes(), " "))

	resp, body, err := c.postForm(ctx, c.deviceCodeEndpoint(), form)
	if err != nil {
		return nil, fmt.Errorf("请求 Device Code 失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求 Device Code 失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result deviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 Device Code 响应失败: %w", err)
	}
	if result.DeviceCode == "" || result.UserCode == "" {
		return nil, errors.New("Device Code 响应缺少必要字段")
	}
	if result.Interval <= 0 {
		result.Interval = 5
	}
	if result.ExpiresIn <= 0 {
		result.ExpiresIn = 900
	}
	return &result, nil
}

// pollDeviceCodeToken 轮询 token 接口，直到用户完成授权或设备代码过期。
func (c *oauthClient) pollDeviceCodeToken(ctx context.Context, deviceCode string, interval, expiresIn int) (*tokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return nil, errors.New("设备代码已过期，请重新发起登录")
		}

		form := url.Values{}
		form.Set("client_id", c.clientID)
		form.Set("grant_type", deviceCodeGrantType)
		form.Set("device_code", deviceCode)

		resp, body, err := c.postForm(ctx, c.tokenEndpoint(), form)
		if err != nil {
			return nil, fmt.Errorf("轮询 Device Code token 失败: %w", err)
		}

		var token tokenResponse
		if err := json.Unmarshal(body, &token); err != nil {
			return nil, fmt.Errorf("解析 Device Code token 响应失败: %w", err)
		}
		if resp.StatusCode == http.StatusOK && token.AccessToken != "" {
			return &token, nil
		}

		switch strings.ToLower(token.Error) {
		case "authorization_pending":
		// 用户还没有完成授权，继续等待。
		case "slow_down":
			// 服务端要求降低轮询频率。
			interval += 5
		case "expired_token":
			return nil, errors.New("设备代码已过期，请重新发起登录")
		case "access_denied":
			return nil, errors.New("用户拒绝了本次授权")
		default:
			if token.Error != "" {
				return nil, fmt.Errorf("换取 token 失败: %s: %s", token.Error, token.ErrorDescription)
			}
			return nil, fmt.Errorf("换取 token 失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
}

// refreshAccessToken 使用 refresh token 静默换取新的 access token。
func (c *oauthClient) refreshAccessToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	if refreshToken == "" {
		return nil, errors.New("refresh token 为空，无法静默刷新")
	}

	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("scope", strings.Join(c.requestScopes(), " "))

	resp, body, err := c.postForm(ctx, c.tokenEndpoint(), form)
	if err != nil {
		return nil, fmt.Errorf("刷新 access token 失败: %w", err)
	}

	var token tokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("解析刷新 token 响应失败: %w", err)
	}
	if resp.StatusCode == http.StatusOK && token.AccessToken != "" {
		return &token, nil
	}
	if token.Error != "" {
		return nil, fmt.Errorf("刷新 access token 失败: %s: %s", token.Error, token.ErrorDescription)
	}
	return nil, fmt.Errorf("刷新 access token 失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// postForm 发送 application/x-www-form-urlencoded POST 请求。
// 返回的 body 已经读取完毕；调用方无需关闭响应体。
func (c *oauthClient) postForm(ctx context.Context, endpoint string, form url.Values) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, err
	}
	return resp, body, nil
}
