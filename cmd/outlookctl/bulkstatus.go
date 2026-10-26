package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

type bulkStatusOptions struct {
	ids     string
	confirm bool
	json    bool
	account string
}

func addBulkStatusFlags(fs *flag.FlagSet, options *bulkStatusOptions) {
	fs.StringVar(&options.ids, "ids", "", "邮件 ID 列表，多个用逗号分隔")
	fs.BoolVar(&options.confirm, "confirm", false, "确认实际执行批量操作")
	fs.BoolVar(&options.json, "json", false, "以 JSON 格式输出")
	fs.StringVar(&options.account, "account", "", "指定账号邮箱或 HomeAccountID")
}

func runMailBulkMarkRead(args []string, isRead bool) error {
	fs := flag.NewFlagSet("mail bulk-mark-read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options bulkStatusOptions
	addBulkStatusFlags(fs, &options)
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(options.ids)
	if len(ids) == 0 {
		return errors.New("用法: outlookctl mail bulk-mark-read --ids <id1,id2> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, options.account)
	if err != nil {
		return err
	}
	result, err := graph.BulkSetMessagesRead(ctx, login.AccessToken, ids, isRead, options.confirm)
	if err != nil {
		return err
	}
	return printBulkResult(login.Account.Username, result, options.json)
}

func runMailBulkFlag(args []string, flagged bool) error {
	fs := flag.NewFlagSet("mail bulk-flag", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options bulkStatusOptions
	addBulkStatusFlags(fs, &options)
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(options.ids)
	if len(ids) == 0 {
		return errors.New("用法: outlookctl mail bulk-flag --ids <id1,id2> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, options.account)
	if err != nil {
		return err
	}
	result, err := graph.BulkSetMessagesFlag(ctx, login.AccessToken, ids, flagged, options.confirm)
	if err != nil {
		return err
	}
	return printBulkResult(login.Account.Username, result, options.json)
}

func runMailBulkImportance(args []string) error {
	fs := flag.NewFlagSet("mail bulk-importance", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options bulkStatusOptions
	addBulkStatusFlags(fs, &options)
	levelFlag := fs.String("level", "", "重要性: low/normal/high")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(options.ids)
	if len(ids) == 0 || strings.TrimSpace(*levelFlag) == "" {
		return errors.New("用法: outlookctl mail bulk-importance --ids <id1,id2> --level low|normal|high [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, options.account)
	if err != nil {
		return err
	}
	result, err := graph.BulkSetMessagesImportance(ctx, login.AccessToken, ids, *levelFlag, options.confirm)
	if err != nil {
		return err
	}
	return printBulkResult(login.Account.Username, result, options.json)
}

func runMailBulkCategory(args []string, add bool) error {
	fs := flag.NewFlagSet("mail bulk-category", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options bulkStatusOptions
	addBulkStatusFlags(fs, &options)
	categoryFlag := fs.String("category", "", "类别名称")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(options.ids)
	if len(ids) == 0 || strings.TrimSpace(*categoryFlag) == "" {
		return errors.New("用法: outlookctl mail bulk-category-add|bulk-category-remove --ids <id1,id2> --category <名称> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, options.account)
	if err != nil {
		return err
	}
	var result *graph.BulkResult
	if add {
		result, err = graph.BulkAddMessagesCategory(ctx, login.AccessToken, ids, *categoryFlag, options.confirm)
	} else {
		result, err = graph.BulkRemoveMessagesCategory(ctx, login.AccessToken, ids, *categoryFlag, options.confirm)
	}
	if err != nil {
		return err
	}
	return printBulkResult(login.Account.Username, result, options.json)
}
