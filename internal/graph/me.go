// Package graph 封装对 Microsoft Graph 官方 API 的调用。
//
// 当前只实现登录后验证用的 /me 接口；后续邮件、日历、联系人能力会继续
// 放在本包中，并优先使用官方 Graph SDK 或最小必要的 REST 调用。
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// graphBaseURL 是 Microsoft Graph v1.0 的固定入口。
var graphBaseURL = "https://graph.microsoft.com/v1.0"

// httpClient 是所有 Graph REST 调用共用的 HTTP 客户端。
// 设置超时是为了避免网络异常时 CLI 长时间挂起。
var httpClient = &http.Client{Timeout: 30 * time.Second}

// Me 表示 GET /me 返回的最小用户信息。
type Me struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	Mail              string `json:"mail"`
}

// GetMe 调用 Microsoft Graph /me 接口验证 access token 是否可用。
func GetMe(ctx context.Context, accessToken string) (*Me, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphBaseURL+"/me?$select=id,displayName,userPrincipalName,mail", nil)
	if err != nil {
		return nil, fmt.Errorf("创建 Graph /me 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph /me 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 只读取少量响应体用于错误提示，避免异常响应占用过多内存。
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph /me 返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var me Me
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return nil, fmt.Errorf("解析 Graph /me 响应失败: %w", err)
	}
	return &me, nil
}
