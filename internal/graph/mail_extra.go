package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// ListAllMessagesAll 读取全邮箱邮件，用于增量同步失败时的全量回退。
func ListAllMessagesAll(ctx context.Context, accessToken string, max int) ([]Message, error) {
	if max <= 0 {
		max = 500
	}
	if max > 5000 {
		max = 5000
	}
	pageSize := max
	if pageSize > 100 {
		pageSize = 100
	}
	query := url.Values{}
	query.Set("$top", strconv.Itoa(pageSize))
	query.Set("$select", "id,subject,receivedDateTime,isRead,hasAttachments,webLink,bodyPreview,from,toRecipients,ccRecipients,importance,categories")
	endpoint := graphBaseURL + "/me/messages?" + query.Encode()
	messages := make([]Message, 0, pageSize)
	nextURL := endpoint
	for nextURL != "" && len(messages) < max {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("创建全量邮件同步请求失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		resp, err := doGraphRequest(req)
		if err != nil {
			return nil, fmt.Errorf("调用 Graph 全量邮件同步失败: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Graph 全量邮件同步返回异常状态: HTTP %d: %s", resp.StatusCode, string(body))
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取全量邮件同步结果失败: %w", readErr)
		}
		var result struct {
			Value    []Message `json:"value"`
			NextLink string    `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析全量邮件同步结果失败: %w", err)
		}
		for _, message := range result.Value {
			messages = append(messages, message)
			if len(messages) >= max {
				break
			}
		}
		nextURL = result.NextLink
	}
	return messages, nil
}
