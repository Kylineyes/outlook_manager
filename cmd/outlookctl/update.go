package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/update"
)

func runUpdate(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl update <check>")
		return nil
	}
	if len(args) == 0 || args[0] != "check" {
		return errors.New("用法: outlookctl update check [--json]")
	}
	fs := flag.NewFlagSet("update check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args[1:]); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := update.Check(ctx, version)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "当前版本: %s\n", result.Current)
	fmt.Fprintf(os.Stdout, "最新版本: %s\n", result.Latest)
	if result.IsDevelopment {
		fmt.Fprintln(os.Stdout, "当前是开发版本，无法比较更新。")
	} else if result.UpdateAvailable {
		fmt.Fprintln(os.Stdout, "状态: 发现新版本")
	} else {
		fmt.Fprintln(os.Stdout, "状态: 已是最新版本")
	}
	if result.Release != nil && result.Release.HTMLURL != "" {
		fmt.Fprintf(os.Stdout, "Release: %s\n", result.Release.HTMLURL)
	}
	return nil
}
