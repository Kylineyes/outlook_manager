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

// MessageQuery 表示高级邮件查询条件。
type MessageQuery struct {
	Folder        string
	From          string
	To            string
	Subject       string
	Body          string
	Search        string
	Importance    string
	Category      string
	After         string
	Before        string
	UnreadOnly    bool
	HasAttachment bool
	Top           int
}

// QueryMessages 按条件查询邮件。
// 服务端尽量使用 $filter，收件人、主题和正文关键字在本地过滤。
func QueryMessages(ctx context.Context, accessToken string, query MessageQuery) ([]Message, error) {
	if query.Top <= 0 {
		query.Top = 100
	}
	if query.Top > 1000 {
		query.Top = 1000
	}
	pageSize := query.Top
	if pageSize > 100 {
		pageSize = 100
	}
	filters := make([]string, 0, 8)
	if value := strings.TrimSpace(query.After); value != "" {
		filters = append(filters, "receivedDateTime ge "+value)
	}
	if value := strings.TrimSpace(query.Before); value != "" {
		filters = append(filters, "receivedDateTime le "+value)
	}
	if value := strings.TrimSpace(query.From); value != "" {
		filters = append(filters, "from/emailAddress/address eq '"+strings.ReplaceAll(value, "'", "''")+"'")
	}
	if query.UnreadOnly {
		filters = append(filters, "isRead eq false")
	}
	if query.HasAttachment {
		filters = append(filters, "hasAttachments eq true")
	}
	if value := strings.ToLower(strings.TrimSpace(query.Importance)); value != "" {
		switch value {
		case "low", "normal", "high":
			filters = append(filters, "importance eq '"+value+"'")
		default:
			return nil, fmt.Errorf("不支持的重要性: %s", query.Importance)
		}
	}

	values := url.Values{}
	values.Set("$top", strconv.Itoa(pageSize))
	values.Set("$select", "id,subject,receivedDateTime,isRead,hasAttachments,webLink,bodyPreview,from,toRecipients,ccRecipients,importance,categories")
	if len(filters) > 0 {
		values.Set("$filter", strings.Join(filters, " and "))
	}
	if len(filters) == 0 {
		values.Set("$orderby", "receivedDateTime desc")
	}
	folder := strings.TrimSpace(query.Folder)
	endpoint := graphBaseURL + "/me/messages?" + values.Encode()
	if folder != "" {
		endpoint = graphBaseURL + "/me/mailFolders/" + url.PathEscape(folder) + "/messages?" + values.Encode()
	}

	messages := make([]Message, 0, pageSize)
	nextURL := endpoint
	for nextURL != "" && len(messages) < query.Top {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("创建邮件查询请求失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		resp, err := doGraphRequest(req)
		if err != nil {
			return nil, fmt.Errorf("调用 Graph 邮件查询失败: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Graph 邮件查询返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var result struct {
			Value    []Message `json:"value"`
			NextLink string    `json:"@odata.nextLink"`
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取 Graph 邮件查询结果失败: %w", readErr)
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析 Graph 邮件查询结果失败: %w", err)
		}
		for _, message := range result.Value {
			if !matchesMessageQuery(query, message) {
				continue
			}
			messages = append(messages, message)
			if len(messages) >= query.Top {
				break
			}
		}
		nextURL = result.NextLink
	}
	return messages, nil
}

func matchesMessageQuery(query MessageQuery, message Message) bool {
	if value := strings.TrimSpace(query.From); value != "" {
		target := strings.ToLower(value)
		haystack := strings.ToLower(message.From.EmailAddress.Address + "\n" + message.From.EmailAddress.Name)
		if !strings.Contains(haystack, target) {
			return false
		}
	}
	if value := strings.TrimSpace(query.Category); value != "" {
		found := false
		for _, category := range message.Categories {
			if strings.EqualFold(strings.TrimSpace(category), strings.TrimSpace(value)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if value := strings.TrimSpace(query.Subject); value != "" && !strings.Contains(strings.ToLower(message.Subject), strings.ToLower(value)) {
		return false
	}
	bodyText := strings.ToLower(message.BodyPreview)
	if value := strings.TrimSpace(query.Body); value != "" && !strings.Contains(bodyText, strings.ToLower(value)) {
		return false
	}
	if value := strings.TrimSpace(query.Search); value != "" {
		haystack := strings.ToLower(message.Subject + "\n" + message.BodyPreview)
		if !strings.Contains(haystack, strings.ToLower(value)) {
			return false
		}
	}
	if value := strings.TrimSpace(query.To); value != "" {
		found := false
		target := strings.ToLower(value)
		for _, recipient := range message.ToRecipients {
			if strings.Contains(strings.ToLower(recipient.EmailAddress.Address), target) || strings.Contains(strings.ToLower(recipient.EmailAddress.Name), target) {
				found = true
				break
			}
		}
		for _, recipient := range message.CcRecipients {
			if strings.Contains(strings.ToLower(recipient.EmailAddress.Address), target) || strings.Contains(strings.ToLower(recipient.EmailAddress.Name), target) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
