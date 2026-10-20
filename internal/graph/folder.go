package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MailFolder 表示 Outlook 邮箱中的一个邮件文件夹。
type MailFolder struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	ParentFolderID   string `json:"parentFolderId"`
	ChildFolderCount int    `json:"childFolderCount"`
	TotalItemCount   int    `json:"totalItemCount"`
	UnreadItemCount  int    `json:"unreadItemCount"`
}

// mailFolderListResponse 是 Graph 邮件文件夹列表接口的响应结构。
type mailFolderListResponse struct {
	Value []MailFolder `json:"value"`
}

// ListMailFolders 读取当前账号的邮件文件夹列表。
func ListMailFolders(ctx context.Context, accessToken string, top int) ([]MailFolder, error) {
	if top <= 0 {
		top = 50
	}
	if top > 200 {
		top = 200
	}

	query := url.Values{}
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,displayName,parentFolderId,childFolderCount,totalItemCount,unreadItemCount")
	query.Set("$orderby", "displayName")
	endpoint := graphBaseURL + "/me/mailFolders?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建邮件文件夹请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 邮件文件夹列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 邮件文件夹列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result mailFolderListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 邮件文件夹列表失败: %w", err)
	}
	return result.Value, nil
}

// ResolveMailFolderID 尝试把用户输入的文件夹名称或 ID 解析成 Graph 可用的 ID。
// 常见 well-known 名称直接透传；否则按显示名查找，找不到时按 ID 交给 Graph。
func ResolveMailFolderID(ctx context.Context, accessToken, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("目标文件夹不能为空")
	}
	switch strings.ToLower(target) {
	case "inbox", "archive", "deleteditems", "drafts", "sentitems", "junkemail":
		return target, nil
	}
	folders, err := ListMailFolders(ctx, accessToken, 200)
	if err == nil {
		for _, folder := range folders {
			if strings.EqualFold(folder.DisplayName, target) {
				return folder.ID, nil
			}
		}
	}
	return target, nil
}

// CreateMailFolder 创建一个邮件文件夹。
// parentID 为空时创建在根目录下，否则创建在指定父文件夹下。
func CreateMailFolder(ctx context.Context, accessToken, displayName, parentID string) (*MailFolder, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("文件夹名称不能为空")
	}

	payload, err := json.Marshal(map[string]string{"displayName": displayName})
	if err != nil {
		return nil, fmt.Errorf("序列化创建文件夹请求失败: %w", err)
	}

	endpoint := graphBaseURL + "/me/mailFolders"
	if strings.TrimSpace(parentID) != "" {
		endpoint = graphBaseURL + "/me/mailFolders/" + url.PathEscape(parentID) + "/childFolders"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("创建文件夹请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 创建文件夹失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 创建文件夹返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var folder MailFolder
	if err := json.NewDecoder(resp.Body).Decode(&folder); err != nil {
		return nil, fmt.Errorf("解析创建文件夹响应失败: %w", err)
	}
	return &folder, nil
}

// RenameMailFolder 重命名邮件文件夹。
func RenameMailFolder(ctx context.Context, accessToken, folderID, displayName string) error {
	folderID = strings.TrimSpace(folderID)
	displayName = strings.TrimSpace(displayName)
	if folderID == "" {
		return fmt.Errorf("文件夹 ID 不能为空")
	}
	if displayName == "" {
		return fmt.Errorf("文件夹名称不能为空")
	}

	payload, err := json.Marshal(map[string]string{"displayName": displayName})
	if err != nil {
		return fmt.Errorf("序列化重命名文件夹请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/mailFolders/" + url.PathEscape(folderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("创建重命名文件夹请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 重命名文件夹失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 重命名文件夹返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// DeleteMailFolder 删除邮件文件夹。
func DeleteMailFolder(ctx context.Context, accessToken, folderID string) error {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return fmt.Errorf("文件夹 ID 不能为空")
	}

	endpoint := graphBaseURL + "/me/mailFolders/" + url.PathEscape(folderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除文件夹请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除文件夹失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除文件夹返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
