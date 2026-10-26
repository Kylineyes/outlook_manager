package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kylineyes/outlook-manager/internal/contactmerge"
	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/ics"
	"github.com/Kylineyes/outlook-manager/internal/localindex"
)

type calendarExportICSArgs struct {
	File     string `json:"file" jsonschema:"导出的 ICS 文件路径"`
	Calendar string `json:"calendar,omitempty" jsonschema:"日历名称或 ID"`
	Max      int    `json:"max,omitempty" jsonschema:"最多导出事件数量"`
	Account  string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type calendarImportICSArgs struct {
	File    string `json:"file" jsonschema:"要导入的 ICS 文件路径"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"确认创建事件"`
	Max     int    `json:"max,omitempty" jsonschema:"最多导入事件数量"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type calendarSeriesInfoArgs struct {
	ID      string `json:"id" jsonschema:"事件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type calendarSeriesUpdateArgs struct {
	ID          string   `json:"id" jsonschema:"事件 ID"`
	Scope       string   `json:"scope,omitempty" jsonschema:"single 或 series"`
	Subject     string   `json:"subject,omitempty" jsonschema:"事件主题"`
	Start       string   `json:"start,omitempty" jsonschema:"开始时间"`
	End         string   `json:"end,omitempty" jsonschema:"结束时间"`
	TimeZone    string   `json:"timeZone,omitempty" jsonschema:"时区"`
	Location    string   `json:"location,omitempty" jsonschema:"地点"`
	Body        string   `json:"body,omitempty" jsonschema:"正文"`
	Categories  []string `json:"categories,omitempty" jsonschema:"类别列表"`
	ShowAs      string   `json:"showAs,omitempty" jsonschema:"忙碌状态"`
	Importance  string   `json:"importance,omitempty" jsonschema:"重要性"`
	Sensitivity string   `json:"sensitivity,omitempty" jsonschema:"敏感度"`
	Account     string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type calendarSeriesDeleteArgs struct {
	ID      string `json:"id" jsonschema:"事件 ID"`
	Scope   string `json:"scope,omitempty" jsonschema:"single 或 series"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type calendarFreeBusyArgs struct {
	Schedules []string `json:"schedules" jsonschema:"日程邮箱列表"`
	Start     string   `json:"start" jsonschema:"开始时间"`
	End       string   `json:"end" jsonschema:"结束时间"`
	Interval  int      `json:"interval,omitempty" jsonschema:"可用性视图间隔分钟"`
	Account   string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type calendarFindTimeArgs struct {
	Attendees []string `json:"attendees" jsonschema:"参与人邮箱列表"`
	Start     string   `json:"start" jsonschema:"查询开始时间"`
	End       string   `json:"end" jsonschema:"查询结束时间"`
	Duration  int      `json:"duration,omitempty" jsonschema:"会议时长分钟"`
	Max       int      `json:"max,omitempty" jsonschema:"最多候选数量"`
	Account   string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type contactsDuplicatesScanArgs struct {
	Strategy string `json:"strategy,omitempty" jsonschema:"email/phone/name"`
	Folder   string `json:"folder,omitempty" jsonschema:"联系人文件夹名称或 ID"`
	Max      int    `json:"max,omitempty" jsonschema:"最多扫描联系人数量"`
	Account  string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type contactsDuplicatesMergeArgs struct {
	Strategy string `json:"strategy,omitempty" jsonschema:"email/phone/name"`
	Folder   string `json:"folder,omitempty" jsonschema:"联系人文件夹名称或 ID"`
	Max      int    `json:"max,omitempty" jsonschema:"最多扫描联系人数量"`
	Confirm  bool   `json:"confirm,omitempty" jsonschema:"确认更新主联系人并删除重复联系人"`
	Account  string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type syncRunArgs struct {
	Mail     bool   `json:"mail,omitempty" jsonschema:"同步邮件"`
	Contacts bool   `json:"contacts,omitempty" jsonschema:"同步联系人"`
	Events   bool   `json:"events,omitempty" jsonschema:"同步日历事件"`
	Max      int    `json:"max,omitempty" jsonschema:"每个类型最多同步数量"`
	Account  string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID"`
}

type syncResetArgs struct {
	Confirm bool `json:"confirm,omitempty" jsonschema:"确认删除本地索引"`
}

type localSearchArgs struct {
	Query string `json:"query" jsonschema:"搜索关键字"`
	Type  string `json:"type,omitempty" jsonschema:"all/mail/contact/event"`
	Limit int    `json:"limit,omitempty" jsonschema:"最多返回数量"`
}

func registerCalendarExtraTools(server *mcp.Server) {
	addTool(server, newTool("calendar_export_ics", "导出日历事件为 ICS 文件"), calendarExportICSHandler)
	addTool(server, newTool("calendar_import_ics", "从 ICS 文件导入日历事件"), calendarImportICSHandler)
	addTool(server, newTool("calendar_series_info", "查看重复事件系列信息"), calendarSeriesInfoHandler)
	addTool(server, newTool("calendar_series_update", "更新单次事件或整个系列"), calendarSeriesUpdateHandler)
	addTool(server, newTool("calendar_series_delete", "删除单次事件或整个系列"), calendarSeriesDeleteHandler)
	addTool(server, newTool("calendar_free_busy", "查询空闲/忙碌状态"), calendarFreeBusyHandler)
	addTool(server, newTool("calendar_find_meeting_times", "查询共同空闲会议时间"), calendarFindTimeHandler)
}

func registerContactExtraTools(server *mcp.Server) {
	addTool(server, newTool("contacts_duplicates_scan", "扫描重复联系人"), contactsDuplicatesScanHandler)
	addTool(server, newTool("contacts_duplicates_merge", "合并重复联系人"), contactsDuplicatesMergeHandler)
}

func registerIndexTools(server *mcp.Server) {
	addTool(server, newTool("sync_run", "同步本地索引"), syncRunHandler)
	addTool(server, newTool("sync_status", "查看本地索引状态"), syncStatusHandler)
	addTool(server, newTool("sync_reset", "删除本地索引"), syncResetHandler)
	addTool(server, newTool("local_search", "搜索本地索引"), localSearchHandler)
}

func calendarExportICSHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarExportICSArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("导出文件路径不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	calendarID := ""
	if strings.TrimSpace(args.Calendar) != "" {
		calendarID, err = graph.ResolveCalendarID(ctx, login.AccessToken, args.Calendar)
		if err != nil {
			return nil, nil, err
		}
	}
	events, err := graph.ListCalendarEventsAll(ctx, login.AccessToken, calendarID, args.Max)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Create(args.File)
	if err != nil {
		return nil, nil, fmt.Errorf("创建 ICS 文件失败: %w", err)
	}
	defer file.Close()
	if err := ics.WriteEvents(file, events); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "file": args.File, "count": len(events)})
}

func calendarImportICSHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarImportICSArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("ICS 文件路径不能为空")
	}
	file, err := os.Open(args.File)
	if err != nil {
		return nil, nil, err
	}
	events, err := ics.Parse(file)
	file.Close()
	if err != nil {
		return nil, nil, err
	}
	if args.Max > 0 && len(events) > args.Max {
		events = events[:args.Max]
	}
	if !args.Confirm {
		return jsonResult(map[string]any{"success": true, "dryRun": true, "count": len(events)})
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	created := 0
	failures := make([]string, 0)
	for _, event := range events {
		if _, err := graph.CreateCalendarEvent(ctx, login.AccessToken, event); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", event.Subject, err))
			continue
		}
		created++
	}
	return jsonResult(map[string]any{"success": len(failures) == 0, "account": login.Account.Username, "created": created, "failed": len(failures), "failures": failures})
}

func calendarSeriesInfoHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarSeriesInfoArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("事件 ID 不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	event, err := graph.GetCalendarEvent(ctx, login.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "event": event})
}

func calendarSeriesDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarSeriesDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("事件 ID 不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	event, err := graph.GetCalendarEvent(ctx, login.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	targetID, err := graph.ResolveSeriesTarget(event, args.Scope)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.DeleteCalendarEvent(ctx, login.AccessToken, targetID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "deletedId": targetID})
}

func calendarSeriesUpdateHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarSeriesUpdateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("事件 ID 不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	event, err := graph.GetCalendarEvent(ctx, login.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	targetID, err := graph.ResolveSeriesTarget(event, args.Scope)
	if err != nil {
		return nil, nil, err
	}
	start, err := parseMCPEventTime(event.Start)
	if err != nil {
		return nil, nil, err
	}
	end, err := parseMCPEventTime(event.End)
	if err != nil {
		return nil, nil, err
	}
	input := graph.CalendarEventInput{
		Subject: event.Subject, Body: event.Body.Content, Start: start, End: end,
		TimeZone: event.Start.TimeZone, Location: event.Location.DisplayName,
		IsAllDay: event.IsAllDay, Categories: event.Categories, ShowAs: event.ShowAs,
		Importance: event.Importance, Sensitivity: event.Sensitivity,
	}
	if args.Subject != "" {
		input.Subject = args.Subject
	}
	if args.Start != "" {
		parsed, err := parseCalendarTime(args.Start)
		if err != nil {
			return nil, nil, err
		}
		input.Start = parsed
	}
	if args.End != "" {
		parsed, err := parseCalendarTime(args.End)
		if err != nil {
			return nil, nil, err
		}
		input.End = parsed
	}
	if args.TimeZone != "" {
		input.TimeZone = args.TimeZone
	}
	if args.Location != "" {
		input.Location = args.Location
	}
	if args.Body != "" {
		input.Body = args.Body
	}
	if len(args.Categories) > 0 {
		input.Categories = args.Categories
	}
	if args.ShowAs != "" {
		input.ShowAs = args.ShowAs
	}
	if args.Importance != "" {
		input.Importance = args.Importance
	}
	if args.Sensitivity != "" {
		input.Sensitivity = args.Sensitivity
	}
	updated, err := graph.UpdateCalendarEvent(ctx, login.AccessToken, targetID, input)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "updatedId": targetID, "event": updated})
}

func parseMCPEventTime(value graph.DateTimeTimeZone) (time.Time, error) {
	return parseEventDateTime(value)
}

func calendarFreeBusyHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarFreeBusyArgs) (*mcp.CallToolResult, any, error) {
	if len(args.Schedules) == 0 || strings.TrimSpace(args.Start) == "" || strings.TrimSpace(args.End) == "" {
		return nil, nil, fmt.Errorf("schedules、start 和 end 不能为空")
	}
	start, err := parseCalendarTime(args.Start)
	if err != nil {
		return nil, nil, err
	}
	end, err := parseCalendarTime(args.End)
	if err != nil {
		return nil, nil, err
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	result, err := graph.GetSchedule(ctx, login.AccessToken, args.Schedules, start, end, args.Interval)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "schedules": result})
}

func calendarFindTimeHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarFindTimeArgs) (*mcp.CallToolResult, any, error) {
	if len(args.Attendees) == 0 || strings.TrimSpace(args.Start) == "" || strings.TrimSpace(args.End) == "" {
		return nil, nil, fmt.Errorf("attendees、start 和 end 不能为空")
	}
	start, err := parseCalendarTime(args.Start)
	if err != nil {
		return nil, nil, err
	}
	end, err := parseCalendarTime(args.End)
	if err != nil {
		return nil, nil, err
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	result, err := graph.FindMeetingTimes(ctx, login.AccessToken, args.Attendees, start, end, args.Duration, args.Max)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "suggestions": result})
}

func contactsDuplicatesScanHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsDuplicatesScanArgs) (*mcp.CallToolResult, any, error) {
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	contacts, err := loadContactsForMerge(ctx, login.AccessToken, args.Folder, args.Max)
	if err != nil {
		return nil, nil, err
	}
	groups := contactmerge.FindGroups(contacts, args.Strategy)
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "totalContacts": len(contacts), "groups": groups})
}

func contactsDuplicatesMergeHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsDuplicatesMergeArgs) (*mcp.CallToolResult, any, error) {
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	contacts, err := loadContactsForMerge(ctx, login.AccessToken, args.Folder, args.Max)
	if err != nil {
		return nil, nil, err
	}
	groups := contactmerge.FindGroups(contacts, args.Strategy)
	plans := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		master, duplicateIDs := contactmerge.MergeGroup(group)
		plans = append(plans, map[string]any{"key": group.Key, "masterId": master.ID, "masterName": master.DisplayName, "duplicateIds": duplicateIDs})
	}
	if !args.Confirm {
		return jsonResult(map[string]any{"success": true, "dryRun": true, "account": login.Account.Username, "groups": plans})
	}
	updated, deleted := 0, 0
	failures := make([]string, 0)
	for _, group := range groups {
		master, duplicateIDs := contactmerge.MergeGroup(group)
		if _, err := graph.UpdateContact(ctx, login.AccessToken, master.ID, graph.ContactToInput(master)); err != nil {
			failures = append(failures, fmt.Sprintf("更新 %s 失败: %v", master.ID, err))
			continue
		}
		updated++
		for _, duplicateID := range duplicateIDs {
			if err := graph.DeleteContact(ctx, login.AccessToken, duplicateID); err != nil {
				failures = append(failures, fmt.Sprintf("删除 %s 失败: %v", duplicateID, err))
				continue
			}
			deleted++
		}
	}
	return jsonResult(map[string]any{"success": len(failures) == 0, "account": login.Account.Username, "updated": updated, "deleted": deleted, "failures": failures})
}

func syncRunHandler(ctx context.Context, req *mcp.CallToolRequest, args syncRunArgs) (*mcp.CallToolResult, any, error) {
	all := !args.Mail && !args.Contacts && !args.Events
	options := localindex.SyncOptions{Messages: all || args.Mail, Contacts: all || args.Contacts, Events: all || args.Events, Max: args.Max}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := localindex.Load()
	if err != nil {
		return nil, nil, err
	}
	report, err := localindex.Sync(ctx, login.AccessToken, snapshot, options)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "report": report})
}

func syncStatusHandler(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
	snapshot, err := localindex.Load()
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "version": snapshot.Version, "syncedAt": snapshot.SyncedAt, "messages": len(snapshot.Messages), "contacts": len(snapshot.Contacts), "events": len(snapshot.Events)})
}

func syncResetHandler(ctx context.Context, req *mcp.CallToolRequest, args syncResetArgs) (*mcp.CallToolResult, any, error) {
	if !args.Confirm {
		return nil, nil, fmt.Errorf("删除本地索引需要 confirm=true")
	}
	path, err := localindex.IndexPath()
	if err != nil {
		return nil, nil, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "path": path})
}

func localSearchHandler(ctx context.Context, req *mcp.CallToolRequest, args localSearchArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Query) == "" {
		return nil, nil, fmt.Errorf("搜索关键字不能为空")
	}
	snapshot, err := localindex.Load()
	if err != nil {
		return nil, nil, err
	}
	hits := localindex.Search(snapshot, args.Query, args.Type, args.Limit)
	return jsonResult(map[string]any{"success": true, "count": len(hits), "hits": hits})
}

func parseCalendarTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", value)
}

func parseEventDateTime(value graph.DateTimeTimeZone) (time.Time, error) {
	location := time.UTC
	if strings.TrimSpace(value.TimeZone) != "" {
		if loaded, err := time.LoadLocation(value.TimeZone); err == nil {
			location = loaded
		}
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value.DateTime, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析事件时间: %s", value.DateTime)
}

func loadContactsForMerge(ctx context.Context, accessToken, folder string, max int) ([]graph.Contact, error) {
	folderID := ""
	var err error
	if strings.TrimSpace(folder) != "" {
		folderID, err = graph.ResolveContactFolderID(ctx, accessToken, folder)
		if err != nil {
			return nil, err
		}
	}
	if max <= 0 {
		max = 1000
	}
	return graph.ListContactsInFolderAll(ctx, accessToken, folderID, max)
}
