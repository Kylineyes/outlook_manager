package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DeltaResult 表示一次增量同步结果。
type DeltaResult[T any] struct {
	Items     []T
	Removed   []string
	DeltaLink string
}

type deltaEnvelope struct {
	Value     []json.RawMessage `json:"value"`
	NextLink  string            `json:"@odata.nextLink"`
	DeltaLink string            `json:"@odata.deltaLink"`
}

func runDelta[T any](ctx context.Context, accessToken, initialURL string, max int, prefer string) (*DeltaResult[T], error) {
	if max <= 0 {
		max = 500
	}
	result := &DeltaResult[T]{Items: make([]T, 0), Removed: make([]string, 0)}
	nextURL := initialURL
	for nextURL != "" && len(result.Items) < max {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("创建增量同步请求失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		if prefer != "" {
			req.Header.Set("Prefer", prefer)
		}
		resp, err := doGraphRequest(req)
		if err != nil {
			return nil, fmt.Errorf("调用 Graph 增量同步失败: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Graph 增量同步返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取增量同步结果失败: %w", readErr)
		}
		var envelope deltaEnvelope
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("解析增量同步结果失败: %w", err)
		}
		for _, raw := range envelope.Value {
			var meta struct {
				ID      string `json:"id"`
				Removed *struct {
					Reason string `json:"reason"`
				} `json:"@removed"`
			}
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, fmt.Errorf("解析增量条目元数据失败: %w", err)
			}
			if meta.Removed != nil {
				if meta.ID != "" {
					result.Removed = append(result.Removed, meta.ID)
				}
				continue
			}
			var item T
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, fmt.Errorf("解析增量条目失败: %w", err)
			}
			result.Items = append(result.Items, item)
			if len(result.Items) >= max {
				break
			}
		}
		if envelope.DeltaLink != "" {
			result.DeltaLink = envelope.DeltaLink
			break
		}
		nextURL = envelope.NextLink
	}
	return result, nil
}

// DeltaMessages 增量同步邮件。
func DeltaMessages(ctx context.Context, accessToken, deltaLink string, max int) (*DeltaResult[Message], error) {
	endpoint := deltaLink
	if strings.TrimSpace(endpoint) == "" {
		endpoint = graphBaseURL + "/me/messages/delta"
	}
	return runDelta[Message](ctx, accessToken, endpoint, max, "odata.maxpagesize=100")
}

// DeltaContacts 增量同步联系人。
func DeltaContacts(ctx context.Context, accessToken, deltaLink string, max int) (*DeltaResult[Contact], error) {
	endpoint := deltaLink
	if strings.TrimSpace(endpoint) == "" {
		endpoint = graphBaseURL + "/me/contacts/delta"
	}
	return runDelta[Contact](ctx, accessToken, endpoint, max, "odata.maxpagesize=100")
}

// DeltaEvents 增量同步日历事件。
func DeltaEvents(ctx context.Context, accessToken, deltaLink string, max int) (*DeltaResult[CalendarEvent], error) {
	endpoint := deltaLink
	if strings.TrimSpace(endpoint) == "" {
		endpoint = graphBaseURL + "/me/events/delta"
	}
	return runDelta[CalendarEvent](ctx, accessToken, endpoint, max, `outlook.timezone="UTC", odata.maxpagesize=100`)
}
