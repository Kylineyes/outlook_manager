package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Kylineyes/outlook-manager/internal/audit"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// CreateDraft 创建一封草稿邮件，并返回草稿 ID。
func CreateDraft(ctx context.Context, accessToken string, input SendMailInput) (string, error) {
	message, err := buildMessage(input)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("序列化草稿邮件失败: %w", err)
	}

	endpoint := graphBaseURL + "/me/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("创建草稿请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return "", fmt.Errorf("调用 Graph 创建草稿失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("Graph 创建草稿返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析草稿响应失败: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("草稿响应缺少 id")
	}
	if err := uploadLargeAttachmentsToDraft(ctx, accessToken, result.ID, input.Attachments); err != nil {
		_ = DeleteMessage(ctx, accessToken, result.ID)
		return "", err
	}
	return result.ID, nil
}

// ListDrafts 读取草稿箱中的邮件列表。
func ListDrafts(ctx context.Context, accessToken string, top int) ([]Message, error) {
	if top <= 0 {
		top = 10
	}
	if top > 100 {
		top = 100
	}

	query := url.Values{}
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,subject,receivedDateTime,isRead,hasAttachments,webLink,bodyPreview,from")
	query.Set("$orderby", "lastModifiedDateTime desc")
	endpoint := graphBaseURL + "/me/mailFolders/drafts/messages?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建草稿列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 草稿列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 草稿列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result messageListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 草稿列表失败: %w", err)
	}
	return result.Value, nil
}

// SendDraft 发送一封草稿邮件。
func SendDraft(ctx context.Context, accessToken, messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("草稿 ID 不能为空")
	}

	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建发送草稿请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 发送草稿失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 发送草稿返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	audit.Log("mail.draft.send", map[string]any{"draftId": messageID})
	return nil
}

// UpdateDraft 更新一封草稿邮件。
func UpdateDraft(ctx context.Context, accessToken, draftID string, input SendMailInput) (*MessageDetail, error) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return nil, fmt.Errorf("草稿 ID 不能为空")
	}
	message, err := buildMessage(input)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("序列化草稿更新请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(draftID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建草稿更新请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 更新草稿失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 更新草稿返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var detail MessageDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("解析更新草稿响应失败: %w", err)
	}
	if err := uploadLargeAttachmentsToDraft(ctx, accessToken, draftID, input.Attachments); err != nil {
		return nil, err
	}
	return &detail, nil
}
