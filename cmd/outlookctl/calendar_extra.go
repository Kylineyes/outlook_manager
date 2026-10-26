package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/ics"
)

func calendarLogin(ctx context.Context, account string) (*graphAccountToken, error) {
	_, login, err := mailboxLogin(ctx, account)
	if err != nil {
		return nil, err
	}
	return &graphAccountToken{Username: login.Account.Username, AccessToken: login.AccessToken}, nil
}

type graphAccountToken struct {
	Username    string
	AccessToken string
}

func runCalendarExportICS(args []string) error {
	fs := flag.NewFlagSet("calendar export-ics", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outFlag := fs.String("out", "", "导出的 ICS 文件路径")
	calendarFlag := fs.String("calendar", "", "日历名称或 ID；不填时使用默认日历")
	maxFlag := fs.Int("max", 500, "最多导出多少个事件")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*outFlag) == "" {
		return errors.New("用法: outlookctl calendar export-ics --out <文件> [--calendar <名称>]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	login, err := calendarLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	calendarID := ""
	if strings.TrimSpace(*calendarFlag) != "" {
		calendarID, err = graph.ResolveCalendarID(ctx, login.AccessToken, *calendarFlag)
		if err != nil {
			return err
		}
	}
	events, err := graph.ListCalendarEventsAll(ctx, login.AccessToken, calendarID, *maxFlag)
	if err != nil {
		return err
	}
	file, err := os.Create(*outFlag)
	if err != nil {
		return fmt.Errorf("创建 ICS 文件失败: %w", err)
	}
	defer file.Close()
	if err := ics.WriteEvents(file, events); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Username)
	fmt.Fprintf(os.Stdout, "事件已导出: %d\n", len(events))
	fmt.Fprintf(os.Stdout, "文件: %s\n", *outFlag)
	return nil
}

func runCalendarImportICS(args []string) error {
	fs := flag.NewFlagSet("calendar import-ics", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fileFlag := fs.String("file", "", "要导入的 ICS 文件路径")
	confirmFlag := fs.Bool("confirm", false, "确认创建日历事件；不填时只 dry-run")
	maxFlag := fs.Int("max", 100, "最多导入多少个事件")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*fileFlag) == "" {
		return errors.New("用法: outlookctl calendar import-ics --file <文件> [--confirm]")
	}
	file, err := os.Open(*fileFlag)
	if err != nil {
		return fmt.Errorf("打开 ICS 文件失败: %w", err)
	}
	events, err := ics.Parse(file)
	file.Close()
	if err != nil {
		return err
	}
	if *maxFlag > 0 && len(events) > *maxFlag {
		events = events[:*maxFlag]
	}
	if !*confirmFlag {
		summaries := make([]map[string]any, 0, len(events))
		for _, event := range events {
			summaries = append(summaries, map[string]any{
				"subject": event.Subject, "start": event.Start, "end": event.End,
				"timeZone": event.TimeZone, "repeatType": event.RepeatType,
			})
		}
		if *jsonFlag {
			return printJSON(map[string]any{"success": true, "dryRun": true, "count": len(events), "events": summaries})
		}
		fmt.Fprintln(os.Stdout, "模式: dry-run，未创建事件；确认后加 --confirm 执行")
		fmt.Fprintf(os.Stdout, "事件数量: %d\n", len(events))
		for _, event := range events {
			fmt.Fprintf(os.Stdout, "- %s | %s\n", event.Start.Format("2006-01-02 15:04"), event.Subject)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	login, err := calendarLogin(ctx, *accountFlag)
	if err != nil {
		return err
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
	result := map[string]any{"success": true, "dryRun": false, "account": login.Username, "created": created, "failed": len(failures), "failures": failures}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Username)
	fmt.Fprintf(os.Stdout, "导入成功: %d\n", created)
	fmt.Fprintf(os.Stdout, "导入失败: %d\n", len(failures))
	for _, failure := range failures {
		fmt.Fprintf(os.Stderr, "失败: %s\n", failure)
	}
	return nil
}

func runCalendarSeriesInfo(args []string) error {
	fs := flag.NewFlagSet("calendar series-info", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl calendar series-info --id <事件ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	login, err := calendarLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	event, err := graph.GetCalendarEvent(ctx, login.AccessToken, *idFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(event)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Username)
	fmt.Fprintf(os.Stdout, "事件: %s\n", event.Subject)
	fmt.Fprintf(os.Stdout, "类型: %s\n", event.Type)
	fmt.Fprintf(os.Stdout, "系列主事件: %s\n", event.SeriesMasterID)
	if event.Recurrence != nil {
		fmt.Fprintf(os.Stdout, "重复模式: %s / 间隔 %d\n", event.Recurrence.Pattern.Type, event.Recurrence.Pattern.Interval)
		fmt.Fprintf(os.Stdout, "重复范围: %s / 次数 %d\n", event.Recurrence.Range.Type, event.Recurrence.Range.NumberOfOccurrences)
	} else {
		fmt.Fprintln(os.Stdout, "重复模式: 无")
	}
	return nil
}

func runCalendarSeriesDelete(args []string) error {
	fs := flag.NewFlagSet("calendar series-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	scopeFlag := fs.String("scope", "single", "范围: single 或 series")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl calendar series-delete --id <事件ID> --scope single|series")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	login, err := calendarLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	event, err := graph.GetCalendarEvent(ctx, login.AccessToken, *idFlag)
	if err != nil {
		return err
	}
	targetID, err := graph.ResolveSeriesTarget(event, *scopeFlag)
	if err != nil {
		return err
	}
	if err := graph.DeleteCalendarEvent(ctx, login.AccessToken, targetID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Username)
	fmt.Fprintf(os.Stdout, "已删除事件: %s\n", targetID)
	return nil
}

func runCalendarSeriesUpdate(args []string) error {
	fs := flag.NewFlagSet("calendar series-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	scopeFlag := fs.String("scope", "single", "范围: single 或 series")
	subjectFlag := fs.String("subject", "", "事件主题")
	startFlag := fs.String("start", "", "开始时间")
	endFlag := fs.String("end", "", "结束时间")
	timeZoneFlag := fs.String("timezone", "", "时区")
	locationFlag := fs.String("location", "", "地点")
	bodyFlag := fs.String("body", "", "正文")
	categoriesFlag := fs.String("categories", "", "类别，逗号分隔")
	showAsFlag := fs.String("show-as", "", "忙碌状态")
	importanceFlag := fs.String("importance", "", "重要性")
	sensitivityFlag := fs.String("sensitivity", "", "敏感度")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl calendar series-update --id <事件ID> --scope single|series [--subject ...]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	login, err := calendarLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	event, err := graph.GetCalendarEvent(ctx, login.AccessToken, *idFlag)
	if err != nil {
		return err
	}
	targetID, err := graph.ResolveSeriesTarget(event, *scopeFlag)
	if err != nil {
		return err
	}
	start, err := parseEventDateTime(event.Start)
	if err != nil {
		return err
	}
	end, err := parseEventDateTime(event.End)
	if err != nil {
		return err
	}
	input := graph.CalendarEventInput{
		Subject: event.Subject, Body: event.Body.Content, Start: start, End: end,
		TimeZone: event.Start.TimeZone, Location: event.Location.DisplayName,
		IsAllDay: event.IsAllDay, Categories: event.Categories, ShowAs: event.ShowAs,
		Importance: event.Importance, Sensitivity: event.Sensitivity,
	}
	if event.IsReminderOn {
		input.ReminderMinutesBeforeStart = event.ReminderMinutesBeforeStart
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if visited["subject"] {
		input.Subject = *subjectFlag
	}
	if visited["start"] {
		parsed, err := parseCalendarTime(*startFlag)
		if err != nil {
			return err
		}
		input.Start = parsed
	}
	if visited["end"] {
		parsed, err := parseCalendarTime(*endFlag)
		if err != nil {
			return err
		}
		input.End = parsed
	}
	if visited["timezone"] {
		input.TimeZone = *timeZoneFlag
	}
	if visited["location"] {
		input.Location = *locationFlag
	}
	if visited["body"] {
		input.Body = *bodyFlag
	}
	if visited["categories"] {
		input.Categories = nil
		for _, category := range strings.Split(*categoriesFlag, ",") {
			if strings.TrimSpace(category) != "" {
				input.Categories = append(input.Categories, strings.TrimSpace(category))
			}
		}
	}
	if visited["show-as"] {
		input.ShowAs = *showAsFlag
	}
	if visited["importance"] {
		input.Importance = *importanceFlag
	}
	if visited["sensitivity"] {
		input.Sensitivity = *sensitivityFlag
	}
	updated, err := graph.UpdateCalendarEvent(ctx, login.AccessToken, targetID, input)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(updated)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Username)
	fmt.Fprintf(os.Stdout, "已更新事件: %s\n", targetID)
	return nil
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

func runCalendarFreeBusy(args []string) error {
	fs := flag.NewFlagSet("calendar freebusy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	schedulesFlag := fs.String("schedules", "", "邮箱列表，逗号分隔")
	startFlag := fs.String("start", "", "开始时间")
	endFlag := fs.String("end", "", "结束时间")
	intervalFlag := fs.Int("interval", 30, "可用性视图间隔分钟")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	schedules := splitAddresses(*schedulesFlag)
	if len(schedules) == 0 || strings.TrimSpace(*startFlag) == "" || strings.TrimSpace(*endFlag) == "" {
		return errors.New("用法: outlookctl calendar freebusy --schedules a@x,b@x --start <时间> --end <时间>")
	}
	start, err := parseCalendarTime(*startFlag)
	if err != nil {
		return err
	}
	end, err := parseCalendarTime(*endFlag)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	login, err := calendarLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	result, err := graph.GetSchedule(ctx, login.AccessToken, schedules, start, end, *intervalFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Username)
	for _, schedule := range result {
		fmt.Fprintf(os.Stdout, "%s: %s\n", schedule.ScheduleID, schedule.AvailabilityView)
		for _, item := range schedule.ScheduleItems {
			fmt.Fprintf(os.Stdout, "  - %s | %s - %s | %s\n", item.Status, item.Start.DateTime, item.End.DateTime, item.Subject)
		}
	}
	return nil
}

func runCalendarFindTime(args []string) error {
	fs := flag.NewFlagSet("calendar find-time", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	attendeesFlag := fs.String("attendees", "", "参与人邮箱，逗号分隔")
	startFlag := fs.String("start", "", "查询开始时间")
	endFlag := fs.String("end", "", "查询结束时间")
	durationFlag := fs.Int("duration", 60, "会议时长分钟")
	maxFlag := fs.Int("max", 10, "最多候选数量")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	attendees := splitAddresses(*attendeesFlag)
	if len(attendees) == 0 || strings.TrimSpace(*startFlag) == "" || strings.TrimSpace(*endFlag) == "" {
		return errors.New("用法: outlookctl calendar find-time --attendees a@x,b@x --start <时间> --end <时间>")
	}
	start, err := parseCalendarTime(*startFlag)
	if err != nil {
		return err
	}
	end, err := parseCalendarTime(*endFlag)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	login, err := calendarLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	result, err := graph.FindMeetingTimes(ctx, login.AccessToken, attendees, start, end, *durationFlag, *maxFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Username)
	for _, suggestion := range result {
		fmt.Fprintf(os.Stdout, "- %s - %s | 置信度 %.0f%% | %s\n", suggestion.MeetingTimeSlot.Start.DateTime, suggestion.MeetingTimeSlot.End.DateTime, suggestion.Confidence*100, suggestion.SuggestionReason)
	}
	return nil
}
