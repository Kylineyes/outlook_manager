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
		fmt.Fprintln(os.Stdout, "用法: outlookctl sync <run|status|reset>")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl sync <run|status|reset>")
	}
	switch args[0] {
	case "run":
		return runSyncRun(args[1:])
	case "status":
		return runSyncStatus(args[1:])
	case "reset":
		return runSyncReset(args[1:])
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
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	all := !*mailFlag && !*contactsFlag && !*eventsFlag
	options := localindex.SyncOptions{Messages: all || *mailFlag, Contacts: all || *contactsFlag, Events: all || *eventsFlag, Max: *maxFlag}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	snapshot, err := localindex.Load()
	if err != nil {
		return err
	}
	report, err := localindex.Sync(ctx, login.AccessToken, snapshot, options)
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
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	snapshot, err := localindex.Load()
	if err != nil {
		return err
	}
	status := map[string]any{
		"version":      snapshot.Version,
		"syncedAt":     snapshot.SyncedAt,
		"messages":     len(snapshot.Messages),
		"contacts":     len(snapshot.Contacts),
		"events":       len(snapshot.Events),
		"messageDelta": snapshot.MessageDelta != "",
		"contactDelta": snapshot.ContactDelta != "",
		"eventDelta":   snapshot.EventDelta != "",
	}
	if *jsonFlag {
		return printJSON(status)
	}
	fmt.Fprintf(os.Stdout, "索引版本: %d\n", snapshot.Version)
	fmt.Fprintf(os.Stdout, "最后同步: %s\n", snapshot.SyncedAt.Format(time.RFC3339))
	fmt.Fprintf(os.Stdout, "邮件: %d | delta=%v\n", len(snapshot.Messages), snapshot.MessageDelta != "")
	fmt.Fprintf(os.Stdout, "联系人: %d | delta=%v\n", len(snapshot.Contacts), snapshot.ContactDelta != "")
	fmt.Fprintf(os.Stdout, "日历: %d | delta=%v\n", len(snapshot.Events), snapshot.EventDelta != "")
	return nil
}

func runSyncReset(args []string) error {
	fs := flag.NewFlagSet("sync reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	confirmFlag := fs.Bool("confirm", false, "确认删除本地索引")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if !*confirmFlag {
		return errors.New("删除本地索引需要 --confirm")
	}
	path, err := localindex.IndexPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除本地索引失败: %w", err)
	}
	fmt.Fprintf(os.Stdout, "本地索引已删除: %s\n", path)
	return nil
}

func runLocalSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	queryFlag := fs.String("query", "", "搜索关键字")
	typeFlag := fs.String("type", "all", "类型: all/mail/contact/event")
	limitFlag := fs.Int("limit", 20, "最多返回多少条")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*queryFlag) == "" {
		return errors.New("用法: outlookctl search --query <关键字> [--type all|mail|contact|event]")
	}
	snapshot, err := localindex.Load()
	if err != nil {
		return err
	}
	hits := localindex.Search(snapshot, *queryFlag, *typeFlag, *limitFlag)
	if *jsonFlag {
		return printJSON(hits)
	}
	fmt.Fprintf(os.Stdout, "结果数量: %d\n", len(hits))
	for index, hit := range hits {
		fmt.Fprintf(os.Stdout, "%d. [%s] %s | %s\n", index+1, hit.Type, hit.Title, hit.Subtitle)
		if hit.Snippet != "" {
			fmt.Fprintf(os.Stdout, "   %s\n", strings.ReplaceAll(hit.Snippet, "\n", " "))
		}
	}
	return nil
}
