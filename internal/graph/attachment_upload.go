package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// uploadChunkSize 必须是 320 KiB 的整数倍。
const uploadChunkSize = 10 * 320 * 1024

// attachmentUploadSession 表示 Graph 返回的附件上传会话。
type attachmentUploadSession struct {
	UploadURL string `json:"uploadUrl"`
}

// uploadLargeAttachmentsToDraft 把所有大文件附件上传到指定草稿。
func uploadLargeAttachmentsToDraft(ctx context.Context, accessToken, draftID string, attachments []MailAttachmentInput) error {
	for _, attachment := range attachments {
		if attachment.Path == "" || attachment.Size <= simpleAttachmentLimit {
			continue
		}
		uploadURL, err := createAttachmentUploadSession(ctx, accessToken, draftID, attachment)
		if err != nil {
			return err
		}
		if err := uploadAttachmentToSession(ctx, uploadURL, attachment); err != nil {
			return err
		}
	}
	return nil
}

// createAttachmentUploadSession 为草稿邮件创建附件上传会话。
func createAttachmentUploadSession(ctx context.Context, accessToken, draftID string, attachment MailAttachmentInput) (string, error) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return "", fmt.Errorf("草稿 ID 不能为空")
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
		return "", fmt.Errorf("序列化附件上传会话请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(draftID) + "/attachments/createUploadSession"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("创建附件上传会话请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return "", fmt.Errorf("调用 Graph 创建附件上传会话失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("Graph 创建附件上传会话返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var session attachmentUploadSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", fmt.Errorf("解析附件上传会话响应失败: %w", err)
	}
	if strings.TrimSpace(session.UploadURL) == "" {
		return "", fmt.Errorf("附件上传会话缺少 uploadUrl")
	}
	return session.UploadURL, nil
}

// uploadAttachmentToSession 按字节范围把本地文件上传到上传会话。
func uploadAttachmentToSession(ctx context.Context, uploadURL string, attachment MailAttachmentInput) error {
	if strings.TrimSpace(uploadURL) == "" {
		return fmt.Errorf("附件上传地址不能为空")
	}
	file, err := os.Open(attachment.Path)
	if err != nil {
		return fmt.Errorf("打开附件失败: %w", err)
	}
	defer file.Close()
	if attachment.Size <= 0 {
		return fmt.Errorf("附件大小必须大于 0")
	}
	buffer := make([]byte, uploadChunkSize)
	for offset := int64(0); offset < attachment.Size; {
		length := int64(len(buffer))
		if remaining := attachment.Size - offset; remaining < length {
			length = remaining
		}
		chunk := buffer[:length]
		if _, err := file.ReadAt(chunk, offset); err != nil && err != io.EOF {
			return fmt.Errorf("读取附件分片失败: %w", err)
		}
		end := offset + length - 1
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(chunk))
		if err != nil {
			return fmt.Errorf("创建附件分片上传请求失败: %w", err)
		}
		// 上传会话 URL 已经预签名，不能附加 Authorization 头。
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, attachment.Size))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := doGraphRequest(req)
		if err != nil {
			return fmt.Errorf("上传附件分片失败 (offset=%d): %w", offset, err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
			return fmt.Errorf("附件分片上传返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		offset += length
	}
	return nil
}
