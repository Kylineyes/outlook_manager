package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/localindex"
)

func runSync(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl sync <run|status|reset|compact|prune>")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl sync <run|status|reset|compact>")
	}
	switch args[0] {
	case "run":
		return runSyncRun(args[1:])
	case "status":
		return runSyncStatus(args[1:])
	case "reset":
		return runSyncReset(args[1:])
	case "compact":
		return runSyncCompact(args[1:])
	case "prune":
		return runSyncPrune(args[1:])
	default:
		return fmt.Errorf("未知 sync 子命令: %s", args[0])
	}
}

func runSyncRun(args []string) error {
	fs := flag.NewFlagSet("sync run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	mailFlag := fs.Bool("mail", false, "同步邮件")
	contactsFlag := fs.Bool("contacts", false, "同步联系人")
	eventsFlag := fs.Bool("events", false, "同步日历事件")
	maxFlag := fs.Int("max", 1000, "每个类型最多同步多少条")
	fullFlag := fs.Bool("full", false, "强制全量同步，忽略增量水位")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	all := !*mailFlag && !*contactsFlag && !*eventsFlag
	options := localindex.SyncOptions{Messages: all || *mailFlag, Contacts: all || *contactsFlag, Events: all || *eventsFlag, Max: *maxFlag, Full: *fullFlag}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	report, err := localindex.Sync(ctx, login.AccessToken, options, login.Account.Username)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(report)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "同步时间: %s\n", report.SyncedAt.Format(time.RFC3339))
	if options.Messages {
		fmt.Fprintf(os.Stdout, "邮件: %d 条 | %s\n", report.Messages, report.MessageMode)
	}
	if options.Contacts {
		fmt.Fprintf(os.Stdout, "联系人: %d 条 | %s\n", report.Contacts, report.ContactMode)
	}
	if options.Events {
		fmt.Fprintf(os.Stdout, "日历: %d 条 | %s\n", report.Events, report.EventMode)
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(os.Stderr, "警告: %s\n", warning)
	}
	return nil
}

func runSyncStatus(args []string) error {
	fs := flag.NewFlagSet("sync status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "只查看指定账号")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	stats, err := localindex.LoadStats(*accountFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(stats)
	}
	fmt.Fprintf(os.Stdout, "索引版本: %d\n", stats.Version)
	fmt.Fprintf(os.Stdout, "账号数量: %d\n", len(stats.Accounts))
	for _, item := range stats.Accounts {
		fmt.Fprintf(os.Stdout, "- %s\n", item.Account)
		fmt.Fprintf(os.Stdout, "  最后同步: %s\n", item.SyncedAt.Format(time.RFC3339))
		fmt.Fprintf(os.Stdout, "  邮件: %d | delta=%v\n", item.Messages, item.MessageDelta)
		fmt.Fprintf(os.Stdout, "  联系人: %d | delta=%v\n", item.Contacts, item.ContactDelta)
		fmt.Fprintf(os.Stdout, "  日历: %d | delta=%v\n", item.Events, item.EventDelta)
	}
	fmt.Fprintf(os.Stdout, "总计: 邮件=%d 联系人=%d 日历=%d\n", stats.TotalMessages, stats.TotalContacts, stats.TotalEvents)
	return nil
}

func runSyncReset(args []string) error {
	fs := flag.NewFlagSet("sync reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "只删除指定账号的索引")
	allFlag := fs.Bool("all", false, "删除全部账号的索引数据库")
	confirmFlag := fs.Bool("confirm", false, "确认删除本地索引")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if !*confirmFlag {
		return errors.New("删除本地索引需要 --confirm")
	}
	if strings.TrimSpace(*accountFlag) != "" {
		if err := localindex.ResetAccount(strings.TrimSpace(*accountFlag)); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "账号索引已删除: %s\n", *accountFlag)
		return nil
	}
	if !*allFlag {
		return errors.New("删除全部索引需要 --all，或使用 --account 删除单账号")
	}
	if err := localindex.ResetAll(); err != nil {
		return err
	}
	path, _ := localindex.IndexPath()
	fmt.Fprintf(os.Stdout, "本地索引数据库已删除: %s\n", path)
	return nil
}

func runSyncCompact(args []string) error {
	fs := flag.NewFlagSet("sync compact", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if err := localindex.Compact(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "本地索引已压缩和优化")
	return nil
}

func runSyncPrune(args []string) error {
	fs := flag.NewFlagSet("sync prune", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "只清理指定账号")
	daysFlag := fs.Int("older-than-days", 90, "删除多少天之前的邮件索引")
	confirmFlag := fs.Bool("confirm", false, "确认删除索引记录")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if !*confirmFlag {
		return errors.New("清理索引需要 --confirm")
	}
	if *daysFlag <= 0 {
		return errors.New("--older-than-days 必须大于 0")
	}
	before := time.Now().UTC().AddDate(0, 0, -*daysFlag)
	result, err := localindex.PruneMessages(*accountFlag, before)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "已删除邮件索引: %d 条\n", result.Deleted)
	fmt.Fprintf(os.Stdout, "截止时间: %s\n", before.Format(time.RFC3339))
	return nil
}

func runLocalSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	queryFlag := fs.String("query", "", "搜索关键字")
	typeFlag := fs.String("type", "all", "类型: all/mail/contact/event")
	limitFlag := fs.Int("limit", 20, "最多返回多少条")
	accountFlag := fs.String("account", "", "只搜索指定账号")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*queryFlag) == "" {
		return errors.New("用法: outlookctl search --query <关键字> [--type all|mail|contact|event]")
	}
	hits, err := localindex.Search(*accountFlag, *queryFlag, *typeFlag, *limitFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(hits)
	}
	fmt.Fprintf(os.Stdout, "结果数量: %d\n", len(hits))
	for index, hit := range hits {
		fmt.Fprintf(os.Stdout, "%d. [%s] [%s] %s | %s\n", index+1, hit.Account, hit.Type, hit.Title, hit.Subtitle)
		if hit.Snippet != "" {
			fmt.Fprintf(os.Stdout, "   %s\n", strings.ReplaceAll(hit.Snippet, "\n", " "))
		}
	}
	return nil
}
