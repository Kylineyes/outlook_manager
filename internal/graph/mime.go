package graph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GetMessageMIME 读取指定邮件的原始 MIME 内容。
func GetMessageMIME(ctx context.Context, accessToken, messageID string) ([]byte, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("邮件 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "/$value"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 MIME 导出请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "message/rfc822, */*")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 导出 MIME 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 导出 MIME 返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 MIME 内容失败: %w", err)
	}
	return data, nil
}
