package graph

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Kylineyes/outlook-manager/internal/audit"
)

// AddDraftAttachment 给草稿邮件添加附件，支持大文件上传会话。
func AddDraftAttachment(ctx context.Context, accessToken, draftID string, attachment MailAttachmentInput) (*Attachment, error) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return nil, fmt.Errorf("草稿 ID 不能为空")
	}
	if strings.TrimSpace(attachment.Name) == "" {
		return nil, fmt.Errorf("附件名称不能为空")
	}
	if attachment.Path != "" && attachment.Size > simpleAttachmentLimit {
		uploadURL, err := createAttachmentUploadSession(ctx, accessToken, draftID, attachment)
		if err != nil {
			return nil, err
		}
		if err := uploadAttachmentToSession(ctx, uploadURL, attachment); err != nil {
			return nil, err
		}
		attachments, err := ListAttachments(ctx, accessToken, draftID)
		if err != nil {
			return nil, err
		}
		for i := len(attachments) - 1; i >= 0; i-- {
			if strings.EqualFold(attachments[i].Name, attachment.Name) {
				audit.Log("mail.draft.attachment.add", map[string]any{"draftId": draftID, "attachmentId": attachments[i].ID, "size": attachments[i].Size})
				return &attachments[i], nil
			}
		}
		return nil, fmt.Errorf("上传完成但未找到新附件")
	}
	if len(attachment.ContentBytes) == 0 {
		return nil, fmt.Errorf("附件内容不能为空")
	}
	contentType := strings.TrimSpace(attachment.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	payload := map[string]any{
		"@odata.type":  "#microsoft.graph.fileAttachment",
		"name":         attachment.Name,
		"contentType":  contentType,
		"contentBytes": base64.StdEncoding.EncodeToString(attachment.ContentBytes),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化草稿附件请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(draftID) + "/attachments"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建草稿附件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 添加草稿附件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 添加草稿附件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result Attachment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析添加草稿附件响应失败: %w", err)
	}
	audit.Log("mail.draft.attachment.add", map[string]any{"draftId": draftID, "attachmentId": result.ID, "size": result.Size})
	return &result, nil
}

// DeleteDraftAttachment 删除草稿邮件中的附件。
func DeleteDraftAttachment(ctx context.Context, accessToken, draftID, attachmentID string) error {
	draftID = strings.TrimSpace(draftID)
	attachmentID = strings.TrimSpace(attachmentID)
	if draftID == "" || attachmentID == "" {
		return fmt.Errorf("草稿 ID 和附件 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(draftID) + "/attachments/" + url.PathEscape(attachmentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除草稿附件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除草稿附件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除草稿附件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	audit.Log("mail.draft.attachment.delete", map[string]any{"draftId": draftID, "attachmentId": attachmentID})
	return nil
}

// ReplaceDraftAttachment 替换草稿附件：先添加新附件，再删除旧附件。
func ReplaceDraftAttachment(ctx context.Context, accessToken, draftID, oldAttachmentID string, attachment MailAttachmentInput) (*Attachment, error) {
	draftID = strings.TrimSpace(draftID)
	oldAttachmentID = strings.TrimSpace(oldAttachmentID)
	if draftID == "" || oldAttachmentID == "" {
		return nil, fmt.Errorf("草稿 ID 和旧附件 ID 不能为空")
	}
	newAttachment, err := AddDraftAttachment(ctx, accessToken, draftID, attachment)
	if err != nil {
		return nil, err
	}
	if err := DeleteDraftAttachment(ctx, accessToken, draftID, oldAttachmentID); err != nil {
		return nil, fmt.Errorf("新附件已添加，但删除旧附件失败: %w", err)
	}
	audit.Log("mail.draft.attachment.replace", map[string]any{"draftId": draftID, "oldAttachmentId": oldAttachmentID, "newAttachmentId": newAttachment.ID})
	return newAttachment, nil
}
