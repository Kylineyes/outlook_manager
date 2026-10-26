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

const calendarEventSelect = "id,subject,start,end,location,organizer,isAllDay,webLink,bodyPreview,categories,showAs,importance,sensitivity,responseStatus,type,seriesMasterId,recurrence,body,attendees,isReminderOn,reminderMinutesBeforeStart,iCalUId"

// GetCalendarEvent 读取单个日历事件详情。
func GetCalendarEvent(ctx context.Context, accessToken, eventID string) (*CalendarEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("事件 ID 不能为空")
	}
	query := url.Values{}
	query.Set("$select", calendarEventSelect)
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID) + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建事件详情请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 事件详情失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 事件详情返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var event CalendarEvent
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return nil, fmt.Errorf("解析事件详情失败: %w", err)
	}
	return &event, nil
}

// ListCalendarEventsAll 读取全部事件，包含重复事件主记录。
func ListCalendarEventsAll(ctx context.Context, accessToken, calendarID string, max int) ([]CalendarEvent, error) {
	if max <= 0 {
		max = 500
	}
	if max > 2000 {
		max = 2000
	}
	pageSize := max
	if pageSize > 100 {
		pageSize = 100
	}
	query := url.Values{}
	query.Set("$top", strconv.Itoa(pageSize))
	query.Set("$select", calendarEventSelect)
	endpoint := graphBaseURL + "/me/events?" + query.Encode()
	if strings.TrimSpace(calendarID) != "" {
		endpoint = graphBaseURL + "/me/calendars/" + url.PathEscape(calendarID) + "/events?" + query.Encode()
	}
	events := make([]CalendarEvent, 0, pageSize)
	nextURL := endpoint
	for nextURL != "" && len(events) < max {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("创建事件列表请求失败: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Prefer", `outlook.timezone="UTC"`)
		resp, err := doGraphRequest(req)
		if err != nil {
			return nil, fmt.Errorf("调用 Graph 事件列表失败: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Graph 事件列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取事件列表失败: %w", readErr)
		}
		var result struct {
			Value    []CalendarEvent `json:"value"`
			NextLink string          `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析事件列表失败: %w", err)
		}
		for _, event := range result.Value {
			events = append(events, event)
			if len(events) >= max {
				break
			}
		}
		nextURL = result.NextLink
	}
	return events, nil
}

// ScheduleItem 表示 getSchedule 返回的忙碌时间段。
type ScheduleItem struct {
	Status    string           `json:"status"`
	Start     DateTimeTimeZone `json:"start"`
	End       DateTimeTimeZone `json:"end"`
	Subject   string           `json:"subject,omitempty"`
	Location  string           `json:"location,omitempty"`
	IsPrivate bool             `json:"isPrivate,omitempty"`
}

// ScheduleInfo 表示某个用户的空闲/忙碌信息。
type ScheduleInfo struct {
	ScheduleID       string         `json:"scheduleId"`
	AvailabilityView string         `json:"availabilityView,omitempty"`
	ScheduleItems    []ScheduleItem `json:"scheduleItems"`
}

// GetSchedule 查询指定用户的空闲/忙碌状态。
func GetSchedule(ctx context.Context, accessToken string, schedules []string, start, end time.Time, interval int) ([]ScheduleInfo, error) {
	if len(schedules) == 0 {
		return nil, fmt.Errorf("至少需要一个日程邮箱")
	}
	if interval <= 0 {
		interval = 30
	}
	payload := map[string]any{
		"schedules":                schedules,
		"startTime":                DateTimeTimeZone{DateTime: start.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		"endTime":                  DateTimeTimeZone{DateTime: end.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
		"availabilityViewInterval": interval,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化空闲查询请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/calendar/getSchedule"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建空闲查询请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 空闲查询失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 空闲查询返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []ScheduleInfo `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析空闲查询结果失败: %w", err)
	}
	return result.Value, nil
}

// AttendeeAvailability 表示会议候选时间中的参与人可用性。
type AttendeeAvailability struct {
	Attendee     EventAttendee `json:"attendee"`
	Availability string        `json:"availability,omitempty"`
}

// MeetingTimeSuggestion 表示 findMeetingTimes 返回的候选会议时间。
type MeetingTimeSuggestion struct {
	Confidence            float64                `json:"confidence"`
	OrganizerAvailability string                 `json:"organizerAvailability,omitempty"`
	AttendeeAvailability  []AttendeeAvailability `json:"attendeeAvailability,omitempty"`
	Locations             []EventLocation        `json:"locations,omitempty"`
	MeetingTimeSlot       struct {
		Start DateTimeTimeZone `json:"start"`
		End   DateTimeTimeZone `json:"end"`
	} `json:"meetingTimeSlot"`
	SuggestionReason string `json:"suggestionReason,omitempty"`
}

// FindMeetingTimes 查询共同空闲的会议时间。
func FindMeetingTimes(ctx context.Context, accessToken string, attendees []string, start, end time.Time, durationMinutes, maxCandidates int) ([]MeetingTimeSuggestion, error) {
	if len(attendees) == 0 {
		return nil, fmt.Errorf("至少需要一个参与人")
	}
	if durationMinutes <= 0 {
		durationMinutes = 60
	}
	if maxCandidates <= 0 {
		maxCandidates = 10
	}
	attendeePayload := make([]map[string]any, 0, len(attendees))
	for _, address := range attendees {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		attendeePayload = append(attendeePayload, map[string]any{
			"type":         "required",
			"emailAddress": map[string]string{"address": address},
		})
	}
	if len(attendeePayload) == 0 {
		return nil, fmt.Errorf("参与人邮箱不能为空")
	}
	payload := map[string]any{
		"attendees": attendeePayload,
		"timeConstraint": map[string]any{
			"timeslots": []map[string]any{{
				"start": DateTimeTimeZone{DateTime: start.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
				"end":   DateTimeTimeZone{DateTime: end.UTC().Format("2006-01-02T15:04:05"), TimeZone: "UTC"},
			}},
		},
		"meetingDuration":         fmt.Sprintf("PT%dM", durationMinutes),
		"maxCandidates":           maxCandidates,
		"isOrganizerOptional":     false,
		"returnSuggestionReasons": true,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化会议时间查询请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/findMeetingTimes"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建会议时间查询请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 会议时间查询失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 会议时间查询返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Suggestions []MeetingTimeSuggestion `json:"meetingTimeSuggestions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析会议时间查询结果失败: %w", err)
	}
	return result.Suggestions, nil
}

// ResolveSeriesTarget 根据操作范围返回实际要操作的 Graph 事件 ID。
func ResolveSeriesTarget(event *CalendarEvent, scope string) (string, error) {
	if event == nil {
		return "", fmt.Errorf("事件不能为空")
	}
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "single":
		return event.ID, nil
	case "series", "all":
		if strings.TrimSpace(event.SeriesMasterID) != "" {
			return event.SeriesMasterID, nil
		}
		if strings.EqualFold(event.Type, "seriesMaster") || event.Recurrence != nil {
			return event.ID, nil
		}
		return "", fmt.Errorf("该事件不是重复事件")
	default:
		return "", fmt.Errorf("不支持的系列操作范围: %s", scope)
	}
}
