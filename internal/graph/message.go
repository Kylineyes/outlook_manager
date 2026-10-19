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
	"time"
)

// ItemBody 表示 Graph 返回的邮件正文。
type ItemBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// Recipient 表示邮件的一个收件人。
type Recipient struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

// MessageDetail 表示单封邮件的完整详情。
// 它比 Message 多出正文、收件人、会话 ID 等字段。
type MessageDetail struct {
	ID                         string      `json:"id"`
	Subject                    string      `json:"subject"`
	ReceivedDateTime           time.Time   `json:"receivedDateTime"`
	IsRead                     bool        `json:"isRead"`
	WebLink                    string      `json:"webLink"`
	BodyPreview                string      `json:"bodyPreview"`
	Body                       ItemBody    `json:"body"`
	From                       MessageFrom `json:"from"`
	ToRecipients               []Recipient `json:"toRecipients"`
	CcRecipients               []Recipient `json:"ccRecipients"`
	BccRecipients              []Recipient `json:"bccRecipients"`
	InternetMessageID          string      `json:"internetMessageId"`
	ConversationID             string      `json:"conversationId"`
	Importance                 string      `json:"importance,omitempty"`
	Sensitivity                string      `json:"sensitivity,omitempty"`
	IsReadReceiptRequested     bool        `json:"isReadReceiptRequested"`
	IsDeliveryReceiptRequested bool        `json:"isDeliveryReceiptRequested"`
}

// GetMessage 根据邮件 ID 读取单封邮件详情。
func GetMessage(ctx context.Context, accessToken, messageID string) (*MessageDetail, error) {
	if strings.TrimSpace(messageID) == "" {
		return nil, fmt.Errorf("邮件 ID 不能为空")
	}

	query := url.Values{}
	query.Set("$select", "id,subject,receivedDateTime,isRead,webLink,bodyPreview,body,from,toRecipients,ccRecipients,bccRecipients,internetMessageId,conversationId,importance,isReadReceiptRequested,isDeliveryReceiptRequested")
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建邮件详情请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 邮件详情失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 邮件详情返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var detail MessageDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("解析 Graph 邮件详情失败: %w", err)
	}
	return &detail, nil
}

// SetMessageRead 修改单封邮件的已读状态。
func SetMessageRead(ctx context.Context, accessToken, messageID string, isRead bool) error {
	if strings.TrimSpace(messageID) == "" {
		return fmt.Errorf("邮件 ID 不能为空")
	}

	payload, err := json.Marshal(map[string]bool{"isRead": isRead})
	if err != nil {
		return fmt.Errorf("序列化邮件更新请求失败: %w", err)
	}

	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("创建邮件更新请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 更新邮件状态失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 更新邮件状态返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
