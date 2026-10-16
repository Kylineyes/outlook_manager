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

// CalendarEventInput 是创建或更新日历事件的输入。
type CalendarEventInput struct {
	Subject  string
	Body     string
	Start    time.Time
	End      time.Time
	TimeZone string
	Location string
	IsAllDay bool
}

// buildCalendarEventPayload 构造 Graph 日历事件请求体。
func buildCalendarEventPayload(input CalendarEventInput) map[string]any {
	timeZone := strings.TrimSpace(input.TimeZone)
	if timeZone == "" {
		timeZone = "UTC"
	}
	start := input.Start
	end := input.End
	if start.IsZero() {
		start = time.Now().UTC()
	}
	if end.IsZero() {
		end = start.Add(time.Hour)
	}
	payload := map[string]any{
		"subject": input.Subject,
		"body": map[string]any{
			"contentType": "Text",
			"content":     input.Body,
		},
		"start": map[string]string{
			"dateTime": start.Format("2006-01-02T15:04:05"),
			"timeZone": timeZone,
		},
		"end": map[string]string{
			"dateTime": end.Format("2006-01-02T15:04:05"),
			"timeZone": timeZone,
		},
		"isAllDay": input.IsAllDay,
	}
	if strings.TrimSpace(input.Location) != "" {
		payload["location"] = map[string]string{"displayName": input.Location}
	}
	return payload
}

// CreateCalendarEvent 创建日历事件。
func CreateCalendarEvent(ctx context.Context, accessToken string, input CalendarEventInput) (*CalendarEvent, error) {
	data, err := json.Marshal(buildCalendarEventPayload(input))
	if err != nil {
		return nil, fmt.Errorf("序列化创建日历事件请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphBaseURL+"/me/events", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建日历事件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 创建日历事件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 创建日历事件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var event CalendarEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return nil, fmt.Errorf("解析创建日历事件响应失败: %w", err)
	}
	return &event, nil
}

// UpdateCalendarEvent 更新日历事件。
func UpdateCalendarEvent(ctx context.Context, accessToken, eventID string, input CalendarEventInput) (*CalendarEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("事件 ID 不能为空")
	}
	data, err := json.Marshal(buildCalendarEventPayload(input))
	if err != nil {
		return nil, fmt.Errorf("序列化更新日历事件请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建更新日历事件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 更新日历事件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 更新日历事件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var event CalendarEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return nil, fmt.Errorf("解析更新日历事件响应失败: %w", err)
	}
	return &event, nil
}

// DeleteCalendarEvent 删除日历事件。
func DeleteCalendarEvent(ctx context.Context, accessToken, eventID string) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return fmt.Errorf("事件 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除日历事件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除日历事件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除日历事件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
