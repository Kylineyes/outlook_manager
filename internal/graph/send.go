package graph

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SendMailInput 是发送邮件所需的输入。
type SendMailInput struct {
	To                     []string
	Cc                     []string
	Bcc                    []string
	Subject                string
	Body                   string
	ContentType            string
	SaveToSentItems        bool
	Attachments            []MailAttachmentInput
	Importance             string
	Sensitivity            string
	RequestReadReceipt     bool
	RequestDeliveryReceipt bool
}

// buildMessage 把发送参数转换为 Microsoft Graph 邮件资源结构。
func buildMessage(input SendMailInput) (map[string]any, error) {
	recipients, err := buildRecipients(input.To, "收件人")
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("至少需要一个收件人")
	}
	ccRecipients, err := buildRecipients(input.Cc, "抄送")
	if err != nil {
		return nil, err
	}
	bccRecipients, err := buildRecipients(input.Bcc, "密送")
	if err != nil {
		return nil, err
	}

	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "Text"
	}
	switch strings.ToLower(contentType) {
	case "text":
		contentType = "Text"
	case "html":
		contentType = "HTML"
	default:
		return nil, fmt.Errorf("不支持的正文类型: %s", input.ContentType)
	}

	message := map[string]any{
		"subject": input.Subject,
		"body": map[string]any{
			"contentType": contentType,
			"content":     input.Body,
		},
		"toRecipients": recipients,
	}
	if len(ccRecipients) > 0 {
		message["ccRecipients"] = ccRecipients
	}
	if strings.TrimSpace(input.Importance) != "" {
		importance := strings.ToLower(strings.TrimSpace(input.Importance))
		switch importance {
		case "low", "normal", "high":
			message["importance"] = importance
		default:
			return nil, fmt.Errorf("不支持的邮件重要性: %s", input.Importance)
		}
	}
	if strings.TrimSpace(input.Sensitivity) != "" {
		sensitivity := strings.ToLower(strings.TrimSpace(input.Sensitivity))
		switch sensitivity {
		case "normal", "personal", "private", "confidential":
			message["sensitivity"] = sensitivity
		default:
			return nil, fmt.Errorf("不支持的邮件敏感度: %s", input.Sensitivity)
		}
	}
	if input.RequestReadReceipt {
		message["isReadReceiptRequested"] = true
	}
	if input.RequestDeliveryReceipt {
		message["isDeliveryReceiptRequested"] = true
	}
	if len(input.Attachments) > 0 {
		attachments := make([]map[string]any, 0, len(input.Attachments))
		for _, attachment := range input.Attachments {
			if strings.TrimSpace(attachment.Name) == "" || len(attachment.ContentBytes) == 0 {
				continue
			}
			contentType := strings.TrimSpace(attachment.ContentType)
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			attachments = append(attachments, map[string]any{
				"@odata.type":  "#microsoft.graph.fileAttachment",
				"name":         attachment.Name,
				"contentType":  contentType,
				"contentBytes": base64.StdEncoding.EncodeToString(attachment.ContentBytes),
			})
		}
		if len(attachments) > 0 {
			message["attachments"] = attachments
		}
	}
	if len(bccRecipients) > 0 {
		message["bccRecipients"] = bccRecipients
	}
	return message, nil
}

// SendMail 使用 Microsoft Graph 发送邮件。
//
// Graph 的 sendMail 接口成功时通常返回 202 Accepted，响应体为空。
func SendMail(ctx context.Context, accessToken string, input SendMailInput) error {
	message, err := buildMessage(input)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"message":         message,
		"saveToSentItems": input.SaveToSentItems,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化发送邮件请求失败: %w", err)
	}

	endpoint := graphBaseURL + "/me/sendMail"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建发送邮件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 发送邮件失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 发送邮件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// buildRecipients 把邮箱字符串列表转换为 Graph 需要的收件人结构。
func buildRecipients(addresses []string, label string) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(addresses))
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		if !strings.Contains(address, "@") {
			return nil, fmt.Errorf("%s邮箱地址不合法: %s", label, address)
		}
		result = append(result, map[string]any{
			"emailAddress": map[string]string{
				"address": address,
			},
		})
	}
	return result, nil
}
