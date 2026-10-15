package graph

import (
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

	resp, err := httpClient.Do(req)
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
