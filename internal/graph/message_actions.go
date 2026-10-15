package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// MoveMessage 把邮件移动到指定文件夹。
// destinationID 可以是文件夹 ID，也可以是服务端支持的 well-known 名称。
func MoveMessage(ctx context.Context, accessToken, messageID, destinationID string) (string, error) {
	messageID = strings.TrimSpace(messageID)
	destinationID = strings.TrimSpace(destinationID)
	if messageID == "" {
		return "", fmt.Errorf("邮件 ID 不能为空")
	}
	if destinationID == "" {
		return "", fmt.Errorf("目标文件夹不能为空")
	}

	payload, err := json.Marshal(map[string]string{"destinationId": destinationID})
	if err != nil {
		return "", fmt.Errorf("序列化移动邮件请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "/move"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("创建移动邮件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 Graph 移动邮件失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("Graph 移动邮件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析移动邮件响应失败: %w", err)
	}
	return result.ID, nil
}

// DeleteMessage 删除邮件。
// Graph 默认把邮件移动到已删除邮件文件夹。
func DeleteMessage(ctx context.Context, accessToken, messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("邮件 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除邮件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除邮件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除邮件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
