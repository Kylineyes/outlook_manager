package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/auth"
	"github.com/Kylineyes/outlook-manager/internal/graph"
)

// runMailbox 处理邮箱设置子命令。
func runMailbox(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		printGroupUsage("mailbox")
		return nil
	}
	if len(args) == 0 {
		printGroupUsage("mailbox")
		return errors.New("mailbox 需要指定子命令: settings、auto-reply 或 timezone")
	}
	switch args[0] {
	case "settings":
		return runMailboxSettings(args[1:])
	case "auto-reply":
		return runMailboxAutoReply(args[1:])
	case "timezone":
		return runMailboxTimezone(args[1:])
	default:
		printGroupUsage("mailbox")
		return fmt.Errorf("未知 mailbox 子命令: %s", args[0])
	}
}

func mailboxLogin(ctx context.Context, account string) (*auth.Client, *auth.LoginResult, error) {
	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	return client, loginResult, nil
}

func runMailboxSettings(args []string) error {
	fs := flag.NewFlagSet("mailbox settings", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	settings, err := graph.GetMailboxSettings(ctx, login.AccessToken)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(settings)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "时区: %s\n", settings.TimeZone)
	fmt.Fprintf(os.Stdout, "语言: %s (%s)\n", settings.Language.DisplayName, settings.Language.Locale)
	fmt.Fprintf(os.Stdout, "自动回复状态: %s\n", settings.AutomaticRepliesSetting.Status)
	if settings.WorkingHours != nil {
		fmt.Fprintf(os.Stdout, "工作时间: %s-%s %s\n", settings.WorkingHours.StartTime, settings.WorkingHours.EndTime, settings.WorkingHours.TimeZone.Name)
	}
	return nil
}

func runMailboxAutoReply(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl mailbox auto-reply <get|set>")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl mailbox auto-reply <get|set>")
	}
	switch args[0] {
	case "get":
		return runMailboxAutoReplyGet(args[1:])
	case "set":
		return runMailboxAutoReplySet(args[1:])
	default:
		return fmt.Errorf("未知 auto-reply 子命令: %s", args[0])
	}
}

func runMailboxAutoReplyGet(args []string) error {
	fs := flag.NewFlagSet("mailbox auto-reply get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	settings, err := graph.GetMailboxSettings(ctx, login.AccessToken)
	if err != nil {
		return err
	}
	reply := settings.AutomaticRepliesSetting
	if *jsonFlag {
		return printJSON(reply)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "状态: %s\n", reply.Status)
	fmt.Fprintf(os.Stdout, "外部收件人范围: %s\n", reply.ExternalAudience)
	if reply.ScheduledStartDateTime != nil {
		fmt.Fprintf(os.Stdout, "开始时间: %s %s\n", reply.ScheduledStartDateTime.DateTime, reply.ScheduledStartDateTime.TimeZone)
	}
	if reply.ScheduledEndDateTime != nil {
		fmt.Fprintf(os.Stdout, "结束时间: %s %s\n", reply.ScheduledEndDateTime.DateTime, reply.ScheduledEndDateTime.TimeZone)
	}
	fmt.Fprintf(os.Stdout, "内部回复:\n%s\n", reply.InternalReplyMessage)
	fmt.Fprintf(os.Stdout, "外部回复:\n%s\n", reply.ExternalReplyMessage)
	return nil
}

func runMailboxAutoReplySet(args []string) error {
	fs := flag.NewFlagSet("mailbox auto-reply set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	statusFlag := fs.String("status", "alwaysEnabled", "状态: alwaysEnabled/disabled/scheduled")
	internalFlag := fs.String("internal-message", "", "组织内自动回复内容")
	externalFlag := fs.String("external-message", "", "组织外自动回复内容")
	externalAudienceFlag := fs.String("external-audience", "all", "组织外收件人: none/contactsOnly/all")
	startFlag := fs.String("start", "", "开始时间，格式 2006-01-02 15:04:05")
	endFlag := fs.String("end", "", "结束时间，格式 2006-01-02 15:04:05")
	timeZoneFlag := fs.String("timezone", "UTC", "计划时间时区")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	setting := graph.AutomaticRepliesSetting{
		Status:               *statusFlag,
		ExternalAudience:     *externalAudienceFlag,
		InternalReplyMessage: *internalFlag,
		ExternalReplyMessage: *externalFlag,
	}
	if strings.EqualFold(*statusFlag, "scheduled") {
		if strings.TrimSpace(*startFlag) == "" || strings.TrimSpace(*endFlag) == "" {
			return errors.New("scheduled 状态需要 --start 和 --end")
		}
		start, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(*startFlag))
		if err != nil {
			return fmt.Errorf("解析开始时间失败: %w", err)
		}
		end, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(*endFlag))
		if err != nil {
			return fmt.Errorf("解析结束时间失败: %w", err)
		}
		setting.ScheduledStartDateTime = &graph.DateTimeTimeZone{DateTime: start.Format("2006-01-02T15:04:05"), TimeZone: *timeZoneFlag}
		setting.ScheduledEndDateTime = &graph.DateTimeTimeZone{DateTime: end.Format("2006-01-02T15:04:05"), TimeZone: *timeZoneFlag}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	settings, err := graph.SetAutomaticReplies(ctx, login.AccessToken, setting)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(settings.AutomaticRepliesSetting)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "自动回复已更新: %s\n", settings.AutomaticRepliesSetting.Status)
	return nil
}

func runMailboxTimezone(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl mailbox timezone <get|set>")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl mailbox timezone <get|set>")
	}
	switch args[0] {
	case "get":
		return runMailboxTimezoneGet(args[1:])
	case "set":
		return runMailboxTimezoneSet(args[1:])
	default:
		return fmt.Errorf("未知 timezone 子命令: %s", args[0])
	}
}

func runMailboxTimezoneGet(args []string) error {
	fs := flag.NewFlagSet("mailbox timezone get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	settings, err := graph.GetMailboxSettings(ctx, login.AccessToken)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(map[string]string{"timeZone": settings.TimeZone})
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "时区: %s\n", settings.TimeZone)
	return nil
}

func runMailboxTimezoneSet(args []string) error {
	fs := flag.NewFlagSet("mailbox timezone set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	valueFlag := fs.String("value", "", "IANA 或 Windows 时区，例如 Asia/Shanghai")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*valueFlag) == "" {
		return errors.New("用法: outlookctl mailbox timezone set --value <时区>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	settings, err := graph.SetMailboxTimeZone(ctx, login.AccessToken, *valueFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(map[string]string{"timeZone": settings.TimeZone})
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "时区已更新: %s\n", settings.TimeZone)
	return nil
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(data))
	return nil
}
