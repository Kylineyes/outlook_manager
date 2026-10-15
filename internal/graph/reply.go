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

// ReplyMessage 回复一封邮件。
func ReplyMessage(ctx context.Context, accessToken, messageID, comment string) error {
	return postMessageAction(ctx, accessToken, messageID, "reply", map[string]any{
		"comment": comment,
	})
}

// ReplyAllMessage 回复全部收件人。
func ReplyAllMessage(ctx context.Context, accessToken, messageID, comment string) error {
	return postMessageAction(ctx, accessToken, messageID, "replyAll", map[string]any{
		"comment": comment,
	})
}

// ForwardMessage 转发一封邮件。
func ForwardMessage(ctx context.Context, accessToken, messageID, comment string, to []string) error {
	recipients, err := buildRecipients(to, "收件人")
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return fmt.Errorf("转发邮件至少需要一个收件人")
	}
	return postMessageAction(ctx, accessToken, messageID, "forward", map[string]any{
		"comment":      comment,
		"toRecipients": recipients,
	})
}

// postMessageAction 调用 Graph 的 reply、replyAll 或 forward action。
func postMessageAction(ctx context.Context, accessToken, messageID, action string, payload map[string]any) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("邮件 ID 不能为空")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化邮件操作请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "/" + action
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建邮件操作请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 邮件操作失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 邮件操作返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
