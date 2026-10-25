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

// addLargeCalendarEventAttachment 使用上传会话给日历事件添加大附件。
func addLargeCalendarEventAttachment(ctx context.Context, accessToken, eventID string, attachment MailAttachmentInput) (*CalendarEventAttachment, error) {
	uploadURL, err := createEventAttachmentUploadSession(ctx, accessToken, eventID, attachment)
	if err != nil {
		return nil, err
	}
	if err := uploadAttachmentToSession(ctx, uploadURL, attachment); err != nil {
		return nil, err
	}
	attachments, err := ListCalendarEventAttachments(ctx, accessToken, eventID)
	if err != nil {
		return nil, err
	}
	for i := len(attachments) - 1; i >= 0; i-- {
		if strings.EqualFold(attachments[i].Name, attachment.Name) {
			return &attachments[i], nil
		}
	}
	return &CalendarEventAttachment{
		Name:        attachment.Name,
		ContentType: attachment.ContentType,
		Size:        int(attachment.Size),
	}, nil
}

// createEventAttachmentUploadSession 为日历事件创建附件上传会话。
func createEventAttachmentUploadSession(ctx context.Context, accessToken, eventID string, attachment MailAttachmentInput) (string, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "", fmt.Errorf("事件 ID 不能为空")
	}
	name := strings.TrimSpace(attachment.Name)
	if name == "" {
		name = "attachment"
	}
	payload := map[string]any{
		"AttachmentItem": map[string]any{
			"attachmentType": "file",
			"name":           name,
			"size":           attachment.Size,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化日历附件上传会话请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID) + "/attachments/createUploadSession"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("创建日历附件上传会话请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return "", fmt.Errorf("调用 Graph 创建日历附件上传会话失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("Graph 创建日历附件上传会话返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var session attachmentUploadSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", fmt.Errorf("解析日历附件上传会话响应失败: %w", err)
	}
	if strings.TrimSpace(session.UploadURL) == "" {
		return "", fmt.Errorf("日历附件上传会话缺少 uploadUrl")
	}
	return session.UploadURL, nil
}
