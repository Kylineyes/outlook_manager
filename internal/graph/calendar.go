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

// DateTimeTimeZone 表示 Graph 的日期时间结构。
type DateTimeTimeZone struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// EventLocation 表示事件地点。
type EventLocation struct {
	DisplayName string `json:"displayName"`
}

// EventOrganizer 表示事件组织者。
type EventOrganizer struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

// CalendarEvent 表示一个日历事件。
type CalendarEvent struct {
	ID          string           `json:"id"`
	Subject     string           `json:"subject"`
	Start       DateTimeTimeZone `json:"start"`
	End         DateTimeTimeZone `json:"end"`
	Location    EventLocation    `json:"location"`
	Organizer   EventOrganizer   `json:"organizer"`
	IsAllDay    bool             `json:"isAllDay"`
	WebLink     string           `json:"webLink"`
	BodyPreview string           `json:"bodyPreview"`
}

// ListCalendarEvents 读取指定时间范围内的日历事件。
func ListCalendarEvents(ctx context.Context, accessToken string, start, end time.Time, top int) ([]CalendarEvent, error) {
	if top <= 0 {
		top = 50
	}
	if top > 200 {
		top = 200
	}

	query := url.Values{}
	query.Set("startDateTime", start.UTC().Format(time.RFC3339))
	query.Set("endDateTime", end.UTC().Format(time.RFC3339))
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,subject,start,end,location,organizer,isAllDay,webLink,bodyPreview")
	query.Set("$orderby", "start/dateTime")
	endpoint := graphBaseURL + "/me/calendarView?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建日历事件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 日历事件失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 日历事件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Value []CalendarEvent `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 日历事件失败: %w", err)
	}
	return result.Value, nil
}
