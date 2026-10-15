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
	ID               string      `json:"id"`
	Subject          string      `json:"subject"`
	ReceivedDateTime time.Time   `json:"receivedDateTime"`
	IsRead           bool        `json:"isRead"`
	WebLink          string      `json:"webLink"`
	BodyPreview      string      `json:"bodyPreview"`
	From             MessageFrom `json:"from"`
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
//
// top 表示最多返回多少封邮件：
//   - 小于等于 0 时使用默认值 10；
//   - 大于 100 时会被限制为 100，避免单次请求过大。
func ListInboxMessages(ctx context.Context, accessToken string, top int) ([]Message, error) {
	if top <= 0 {
		top = 10
	}
	if top > 100 {
		top = 100
	}

	query := url.Values{}
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,subject,receivedDateTime,isRead,webLink,bodyPreview,from")
	query.Set("$orderby", "receivedDateTime desc")
	endpoint := graphBaseURL + "/me/mailFolders/inbox/messages?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建邮件列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
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
