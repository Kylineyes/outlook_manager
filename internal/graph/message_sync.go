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

const messageSyncSelect = "id,subject,receivedDateTime,lastModifiedDateTime,isRead,hasAttachments,webLink,bodyPreview,from,toRecipients,ccRecipients,importance,categories"

// ListMessagesModifiedSince 读取指定时间后修改过的邮件。
func ListMessagesModifiedSince(ctx context.Context, accessToken string, since time.Time, max int) ([]Message, error) {
	return listMessagesSince(ctx, accessToken, "lastModifiedDateTime", since, max)
}

// ListMessagesReceivedSince 读取指定时间后收到的邮件。
func ListMessagesReceivedSince(ctx context.Context, accessToken string, since time.Time, max int) ([]Message, error) {
	return listMessagesSince(ctx, accessToken, "receivedDateTime", since, max)
}

func listMessagesSince(ctx context.Context, accessToken, field string, since time.Time, max int) ([]Message, error) {
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
	query.Set("$filter", field+" ge "+since.UTC().Format(time.RFC3339))
	query.Set("$select", messageSyncSelect)
	query.Set("$top", strconv.Itoa(pageSize))
	endpoint := graphBaseURL + "/me/messages?" + query.Encode()

	messages := make([]Message, 0, pageSize)
	nextURL := endpoint
	for nextURL != "" && len(messages) < max {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("创建增量邮件请求失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		resp, err := doGraphRequest(req)
		if err != nil {
			return nil, fmt.Errorf("调用 Graph 增量邮件失败: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Graph 增量邮件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取增量邮件结果失败: %w", readErr)
		}
		var result struct {
			Value    []Message `json:"value"`
			NextLink string    `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析增量邮件结果失败: %w", err)
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
