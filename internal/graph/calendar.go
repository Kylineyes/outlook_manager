package graph

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Kylineyes/outlook-manager/internal/audit"
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

// EventResponseStatus 表示参与人对日历事件的响应状态。
type EventResponseStatus struct {
	Response string `json:"response,omitempty"`
	Time     string `json:"time,omitempty"`
}

// CalendarEvent 表示一个日历事件。
type CalendarEvent struct {
	ID             string               `json:"id"`
	Subject        string               `json:"subject"`
	Start          DateTimeTimeZone     `json:"start"`
	End            DateTimeTimeZone     `json:"end"`
	Location       EventLocation        `json:"location"`
	Organizer      EventOrganizer       `json:"organizer"`
	IsAllDay       bool                 `json:"isAllDay"`
	WebLink        string               `json:"webLink"`
	BodyPreview    string               `json:"bodyPreview"`
	Categories     []string             `json:"categories,omitempty"`
	ShowAs         string               `json:"showAs,omitempty"`
	Importance     string               `json:"importance,omitempty"`
	Sensitivity    string               `json:"sensitivity,omitempty"`
	ResponseStatus *EventResponseStatus `json:"responseStatus,omitempty"`
}

// ListCalendarEvents 读取默认日历在指定时间范围内的事件。
func ListCalendarEvents(ctx context.Context, accessToken string, start, end time.Time, top int) ([]CalendarEvent, error) {
	return ListCalendarEventsInCalendar(ctx, accessToken, "", start, end, top)
}

// ListCalendarEventsInCalendar 读取指定日历在时间范围内的事件。
// calendarID 为空时读取默认日历。
func ListCalendarEventsInCalendar(ctx context.Context, accessToken, calendarID string, start, end time.Time, top int) ([]CalendarEvent, error) {
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
	query.Set("$select", "id,subject,start,end,location,organizer,isAllDay,webLink,bodyPreview,categories,showAs,importance,sensitivity,responseStatus")
	query.Set("$orderby", "start/dateTime")
	endpoint := graphBaseURL + "/me/calendarView?" + query.Encode()
	if strings.TrimSpace(calendarID) != "" {
		endpoint = graphBaseURL + "/me/calendars/" + url.PathEscape(calendarID) + "/calendarView?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建日历事件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)
	resp, err := doGraphRequest(req)
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

type CalendarEventInput struct {
	Subject                    string
	Body                       string
	Start                      time.Time
	End                        time.Time
	TimeZone                   string
	Location                   string
	IsAllDay                   bool
	Attendees                  []string
	ReminderMinutesBeforeStart int
	RepeatType                 string
	RepeatInterval             int
	RepeatCount                int
	RepeatUntil                string
	Categories                 []string
	ShowAs                     string
	Importance                 string
	Sensitivity                string
}

// buildCalendarEventPayload 构造 Graph 日历事件请求体。
func buildCalendarEventPayload(input CalendarEventInput) (map[string]any, error) {
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
	if len(input.Categories) > 0 {
		payload["categories"] = input.Categories
	}
	if strings.TrimSpace(input.ShowAs) != "" {
		showAs := strings.ToLower(strings.TrimSpace(input.ShowAs))
		switch showAs {
		case "free", "tentative", "busy", "oof", "workingelsewhere", "unknown":
			payload["showAs"] = showAs
		default:
			return nil, fmt.Errorf("不支持的日程状态: %s", input.ShowAs)
		}
	}
	if strings.TrimSpace(input.Importance) != "" {
		importance := strings.ToLower(strings.TrimSpace(input.Importance))
		switch importance {
		case "low", "normal", "high":
			payload["importance"] = importance
		default:
			return nil, fmt.Errorf("不支持的事件重要性: %s", input.Importance)
		}
	}
	if strings.TrimSpace(input.Sensitivity) != "" {
		sensitivity := strings.ToLower(strings.TrimSpace(input.Sensitivity))
		switch sensitivity {
		case "normal", "personal", "private", "confidential":
			payload["sensitivity"] = sensitivity
		default:
			return nil, fmt.Errorf("不支持的事件敏感度: %s", input.Sensitivity)
		}
	}
	if strings.TrimSpace(input.Location) != "" {
		payload["location"] = map[string]string{"displayName": input.Location}
	}
	if len(input.Attendees) > 0 {
		attendees := make([]map[string]any, 0, len(input.Attendees))
		for _, email := range input.Attendees {
			email = strings.TrimSpace(email)
			if email == "" {
				continue
			}
			attendees = append(attendees, map[string]any{
				"emailAddress": map[string]string{"address": email},
				"type":         "required",
			})
		}
		if len(attendees) > 0 {
			payload["attendees"] = attendees
		}
	}
	if input.ReminderMinutesBeforeStart > 0 {
		payload["isReminderOn"] = true
		payload["reminderMinutesBeforeStart"] = input.ReminderMinutesBeforeStart
	}
	if strings.TrimSpace(input.RepeatType) != "" {
		repeatType := strings.ToLower(strings.TrimSpace(input.RepeatType))
		switch repeatType {
		case "daily", "weekly", "monthly", "yearly":
		default:
			return nil, fmt.Errorf("不支持的重复类型: %s", input.RepeatType)
		}
		interval := input.RepeatInterval
		if interval <= 0 {
			interval = 1
		}
		pattern := map[string]any{
			"type":     repeatType,
			"interval": interval,
		}
		switch repeatType {
		case "weekly":
			pattern["daysOfWeek"] = []string{weekdayName(start.Weekday())}
		case "monthly":
			pattern["dayOfMonth"] = start.Day()
		case "yearly":
			pattern["month"] = int(start.Month())
			pattern["dayOfMonth"] = start.Day()
		}
		recurrenceRange := map[string]any{
			"startDate": start.Format("2006-01-02"),
		}
		if input.RepeatCount > 0 {
			recurrenceRange["type"] = "numbered"
			recurrenceRange["numberOfOccurrences"] = input.RepeatCount
		} else {
			recurrenceRange["type"] = "endDate"
			if strings.TrimSpace(input.RepeatUntil) != "" {
				recurrenceRange["endDate"] = strings.TrimSpace(input.RepeatUntil)
			} else {
				recurrenceRange["endDate"] = start.AddDate(0, 0, 90).Format("2006-01-02")
			}
		}
		payload["recurrence"] = map[string]any{
			"pattern": pattern,
			"range":   recurrenceRange,
		}
	}
	return payload, nil
}

// weekdayName 把 Go 的 time.Weekday 转换为 Graph 需要的星期名称。
func weekdayName(day time.Weekday) string {
	switch day {
	case time.Sunday:
		return "sunday"
	case time.Monday:
		return "monday"
	case time.Tuesday:
		return "tuesday"
	case time.Wednesday:
		return "wednesday"
	case time.Thursday:
		return "thursday"
	case time.Friday:
		return "friday"
	default:
		return "saturday"
	}
}

// CreateCalendarEvent 创建日历事件。
func CreateCalendarEvent(ctx context.Context, accessToken string, input CalendarEventInput) (*CalendarEvent, error) {
	payload, err := buildCalendarEventPayload(input)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(payload)
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

	resp, err := doGraphRequest(req)
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
	audit.Log("calendar.create", map[string]any{"eventId": event.ID, "subject": event.Subject})
	return &event, nil
}

// UpdateCalendarEvent 更新日历事件。
func UpdateCalendarEvent(ctx context.Context, accessToken, eventID string, input CalendarEventInput) (*CalendarEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("事件 ID 不能为空")
	}
	payload, err := buildCalendarEventPayload(input)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(payload)
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

	resp, err := doGraphRequest(req)
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
	audit.Log("calendar.update", map[string]any{"eventId": event.ID, "subject": event.Subject})
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
	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除日历事件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除日历事件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	audit.Log("calendar.delete", map[string]any{"eventId": eventID})
	return nil
}

// Calendar 表示一个 Outlook 日历。
type Calendar struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Color             string `json:"color"`
	CanEdit           bool   `json:"canEdit"`
	IsDefaultCalendar bool   `json:"isDefaultCalendar"`
}

// ListCalendars 读取当前账号的日历列表。
func ListCalendars(ctx context.Context, accessToken string) ([]Calendar, error) {
	query := url.Values{}
	query.Set("$select", "id,name,color,canEdit,isDefaultCalendar")
	query.Set("$orderby", "name")
	endpoint := graphBaseURL + "/me/calendars?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建日历列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 日历列表失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 日历列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []Calendar `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 日历列表失败: %w", err)
	}
	return result.Value, nil
}

// ResolveCalendarID 按名称查找日历 ID；找不到时返回原始值交给 Graph 处理。
func ResolveCalendarID(ctx context.Context, accessToken, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil
	}
	calendars, err := ListCalendars(ctx, accessToken)
	if err != nil {
		return "", err
	}
	for _, calendar := range calendars {
		if strings.EqualFold(calendar.Name, target) || calendar.ID == target {
			return calendar.ID, nil
		}
	}
	return target, nil
}

// CalendarEventAttachment 表示日历事件附件。
type CalendarEventAttachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int    `json:"size"`
	IsInline    bool   `json:"isInline"`
}

// ListCalendarEventAttachments 读取日历事件附件列表。
func ListCalendarEventAttachments(ctx context.Context, accessToken, eventID string) ([]CalendarEventAttachment, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("事件 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID) + "/attachments"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建日历附件列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 日历附件列表失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 日历附件列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []CalendarEventAttachment `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 日历附件列表失败: %w", err)
	}
	return result.Value, nil
}

// AddCalendarEventAttachment 给日历事件添加文件附件。
func AddCalendarEventAttachment(ctx context.Context, accessToken, eventID string, attachment MailAttachmentInput) (*CalendarEventAttachment, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("事件 ID 不能为空")
	}
	if strings.TrimSpace(attachment.Name) == "" {
		return nil, fmt.Errorf("附件名称不能为空")
	}
	if attachment.Path != "" && attachment.Size > simpleAttachmentLimit {
		return addLargeCalendarEventAttachment(ctx, accessToken, eventID, attachment)
	}
	if len(attachment.ContentBytes) == 0 {
		return nil, fmt.Errorf("附件内容不能为空")
	}
	contentType := strings.TrimSpace(attachment.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	payload := map[string]any{
		"@odata.type":  "#microsoft.graph.fileAttachment",
		"name":         attachment.Name,
		"contentType":  contentType,
		"contentBytes": base64.StdEncoding.EncodeToString(attachment.ContentBytes),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化日历附件请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID) + "/attachments"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建添加日历附件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 添加日历附件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 添加日历附件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result CalendarEventAttachment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 添加日历附件响应失败: %w", err)
	}
	return &result, nil
}

// GetCalendarEventAttachment 下载日历事件附件内容。
func GetCalendarEventAttachment(ctx context.Context, accessToken, eventID, attachmentID string) ([]byte, string, error) {
	eventID = strings.TrimSpace(eventID)
	attachmentID = strings.TrimSpace(attachmentID)
	if eventID == "" || attachmentID == "" {
		return nil, "", fmt.Errorf("事件 ID 和附件 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID) + "/attachments/" + url.PathEscape(attachmentID) + "/$value"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建日历附件下载请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := doGraphRequest(req)
	if err != nil {
		return nil, "", fmt.Errorf("调用 Graph 日历附件下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("Graph 日历附件下载返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取日历附件内容失败: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// CancelCalendarEvent 取消日历事件并通知参与者。
func CancelCalendarEvent(ctx context.Context, accessToken, eventID, comment string) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return fmt.Errorf("事件 ID 不能为空")
	}
	payload := map[string]any{}
	if strings.TrimSpace(comment) != "" {
		payload["comment"] = strings.TrimSpace(comment)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化取消日历事件请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID) + "/cancel"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建取消日历事件请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 取消日历事件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 取消日历事件返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	audit.Log("calendar.cancel", map[string]any{"eventId": eventID})
	return nil
}

// RespondCalendarEvent 响应日历邀请。
// response 支持 accept、decline、tentative。
func RespondCalendarEvent(ctx context.Context, accessToken, eventID, response, comment string) error {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return fmt.Errorf("事件 ID 不能为空")
	}
	action := ""
	switch strings.ToLower(strings.TrimSpace(response)) {
	case "accept", "accepted":
		action = "accept"
	case "decline", "declined":
		action = "decline"
	case "tentative", "tentativelyaccept", "tentatively-accept":
		action = "tentativelyAccept"
	default:
		return fmt.Errorf("不支持的响应类型: %s", response)
	}
	payload := map[string]any{"sendResponse": true}
	if strings.TrimSpace(comment) != "" {
		payload["comment"] = strings.TrimSpace(comment)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化日历响应请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/events/" + url.PathEscape(eventID) + "/" + action
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建日历响应请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := doGraphRequest(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 日历响应失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 日历响应返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	audit.Log("calendar.respond", map[string]any{"eventId": eventID, "response": response})
	return nil
}
