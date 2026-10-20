package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Attachment 表示邮件附件的基本信息。
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	IsInline    bool   `json:"isInline"`
}

// attachmentListResponse 是 Graph 附件列表接口的响应结构。
type attachmentListResponse struct {
	Value []Attachment `json:"value"`
}

// ListAttachments 读取指定邮件的附件列表。
func ListAttachments(ctx context.Context, accessToken, messageID string) ([]Attachment, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("邮件 ID 不能为空")
	}

	query := url.Values{}
	query.Set("$select", "id,name,contentType,size,isInline")
	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "/attachments?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建附件列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 附件列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 附件列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result attachmentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 附件列表失败: %w", err)
	}
	return result.Value, nil
}

// DownloadAttachment 下载指定附件的内容。
func DownloadAttachment(ctx context.Context, accessToken, messageID, attachmentID string) ([]byte, error) {
	messageID = strings.TrimSpace(messageID)
	attachmentID = strings.TrimSpace(attachmentID)
	if messageID == "" {
		return nil, fmt.Errorf("邮件 ID 不能为空")
	}
	if attachmentID == "" {
		return nil, fmt.Errorf("附件 ID 不能为空")
	}

	endpoint := graphBaseURL + "/me/messages/" + url.PathEscape(messageID) + "/attachments/" + url.PathEscape(attachmentID) + "/$value"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建附件下载请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 附件下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 附件下载返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

// SaveAttachment 下载附件并保存到本地文件。
func SaveAttachment(ctx context.Context, accessToken, messageID, attachmentID, outputPath string) (int, error) {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return 0, fmt.Errorf("输出路径不能为空")
	}
	data, err := DownloadAttachment(ctx, accessToken, messageID, attachmentID)
	if err != nil {
		return 0, err
	}
	if dir := filepath.Dir(outputPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return 0, fmt.Errorf("创建附件输出目录失败: %w", err)
		}
	}
	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return 0, fmt.Errorf("写入附件文件失败: %w", err)
	}
	return len(data), nil
}
