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
	"time"
)

// Message 表示收件箱中的一封邮件。
// 当前只保留 CLI 展示和后续 MCP 工具需要的最小字段。
type Message struct {
	ID                   string      `json:"id"`
	Subject              string      `json:"subject"`
	ReceivedDateTime     time.Time   `json:"receivedDateTime"`
	LastModifiedDateTime time.Time   `json:"lastModifiedDateTime,omitempty"`
	IsRead               bool        `json:"isRead"`
	WebLink              string      `json:"webLink"`
	BodyPreview          string      `json:"bodyPreview"`
	HasAttachments       bool        `json:"hasAttachments"`
	Importance           string      `json:"importance,omitempty"`
	From                 MessageFrom `json:"from"`
	ToRecipients         []Recipient `json:"toRecipients,omitempty"`
	CcRecipients         []Recipient `json:"ccRecipients,omitempty"`
	Categories           []string    `json:"categories,omitempty"`
}

// MessageFrom 表示邮件的发件人信息。
type MessageFrom struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

// EmailAddress 表示 Graph 返回的邮箱地址结构。
type EmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// messageListResponse 是 Graph 消息列表接口的响应结构。
type messageListResponse struct {
	Value []Message `json:"value"`
}

// ListInboxMessages 读取当前账号收件箱中的最新邮件。
func ListInboxMessages(ctx context.Context, accessToken string, top int) ([]Message, error) {
	return ListMessagesInFolder(ctx, accessToken, "inbox", top)
}

// ListMessagesInFolder 读取指定文件夹中的最新邮件。
func ListMessagesInFolder(ctx context.Context, accessToken, folderID string, top int) ([]Message, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		folderID = "inbox"
	}
	if top <= 0 {
		top = 10
	}
	if top > 100 {
		top = 100
	}

	query := url.Values{}
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,subject,receivedDateTime,isRead,hasAttachments,webLink,bodyPreview,from,toRecipients,ccRecipients,importance")
	query.Set("$orderby", "receivedDateTime desc")
	endpoint := graphBaseURL + "/me/mailFolders/" + url.PathEscape(folderID) + "/messages?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建邮件列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 邮件列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 邮件列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result messageListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 邮件列表响应失败: %w", err)
	}
	return result.Value, nil
}

// SearchMessages 使用 Microsoft Graph $search 在全部邮箱文件夹中搜索邮件。
func SearchMessages(ctx context.Context, accessToken, query string, top int) ([]Message, error) {
	return SearchMessagesInFolder(ctx, accessToken, "", query, top)
}

// SearchMessagesInFolder 在指定文件夹中使用 $search 搜索邮件。
// folderID 为空时搜索全部邮箱文件夹。
func SearchMessagesInFolder(ctx context.Context, accessToken, folderID, query string, top int) ([]Message, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键字不能为空")
	}
	if top <= 0 {
		top = 10
	}
	if top > 100 {
		top = 100
	}

	values := url.Values{}
	// Graph 要求 $search 的值是一个带引号的字符串；内部双引号需要转义。
	escaped := strings.ReplaceAll(query, `"`, `\"`)
	values.Set("$search", `"`+escaped+`"`)
	values.Set("$top", strconv.Itoa(top))
	values.Set("$select", "id,subject,receivedDateTime,isRead,hasAttachments,webLink,bodyPreview,from,toRecipients,ccRecipients,importance")

	endpoint := graphBaseURL + "/me/messages?" + values.Encode()
	if strings.TrimSpace(folderID) != "" {
		endpoint = graphBaseURL + "/me/mailFolders/" + url.PathEscape(folderID) + "/messages?" + values.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建邮件搜索请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 邮件搜索失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 邮件搜索返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result messageListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 邮件搜索响应失败: %w", err)
	}
	return result.Value, nil
}

// ListMessagesInFolderAll 分页读取指定文件夹中的邮件，最多返回 max 封。
func ListMessagesInFolderAll(ctx context.Context, accessToken, folderID string, max int) ([]Message, error) {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		folderID = "inbox"
	}
	if max <= 0 {
		max = 500
	}
	if max > 1000 {
		max = 1000
	}
	pageSize := max
	if pageSize > 100 {
		pageSize = 100
	}
	query := url.Values{}
	query.Set("$top", strconv.Itoa(pageSize))
	query.Set("$select", "id,subject,receivedDateTime,isRead,hasAttachments,webLink,bodyPreview,from,toRecipients,ccRecipients,importance")
	query.Set("$orderby", "receivedDateTime desc")
	nextURL := graphBaseURL + "/me/mailFolders/" + url.PathEscape(folderID) + "/messages?" + query.Encode()
	messages := make([]Message, 0, pageSize)
	for nextURL != "" && len(messages) < max {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("创建邮件分页请求失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		resp, err := doGraphRequest(req)
		if err != nil {
			return nil, fmt.Errorf("调用 Graph 邮件分页失败: %w", err)
		}
		var result struct {
			Value    []Message `json:"value"`
			NextLink string    `json:"@odata.nextLink"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Graph 邮件分页返回异常状态: HTTP %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("解析 Graph 邮件分页失败: %w", decodeErr)
		}
		for _, message := range result.Value {
			if len(messages) >= max {
				break
			}
			messages = append(messages, message)
		}
		nextURL = result.NextLink
	}
	return messages, nil
}
