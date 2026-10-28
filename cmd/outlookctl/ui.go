package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

// runUI 提供简单的交互式文本菜单。
func runUI(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl ui [--plain]")
		return nil
	}
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	plainFlag := fs.Bool("plain", false, "使用普通文本菜单")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	plain := *plainFlag
	if !plain {
		info, err := os.Stdout.Stat()
		plain = err != nil || info.Mode()&os.ModeCharDevice == 0
	}
	if !plain {
		fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l")
		defer fmt.Fprint(os.Stdout, "\x1b[?25h\x1b[?1049l")
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		if !plain {
			fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
		}
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, "Outlook Manager 交互菜单")
		fmt.Fprintln(os.Stdout, "  1. 邮件列表")
		fmt.Fprintln(os.Stdout, "  2. 日历列表")
		fmt.Fprintln(os.Stdout, "  3. 联系人列表")
		fmt.Fprintln(os.Stdout, "  4. 本地同步状态")
		fmt.Fprintln(os.Stdout, "  5. 审计统计")
		fmt.Fprintln(os.Stdout, "  6. 更新检查")
		fmt.Fprintln(os.Stdout, "  7. 环境诊断")
		fmt.Fprintln(os.Stdout, "  q. 退出")
		fmt.Fprint(os.Stdout, "请选择: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		switch choice {
		case "1":
			_ = runMailList(nil)
		case "2":
			_ = runCalendarList(nil)
		case "3":
			_ = runContactsList(nil)
		case "4":
			_ = runSyncStatus(nil)
		case "5":
			_ = runAuditStats(nil)
		case "6":
			_ = runUpdate([]string{"check"})
		case "7":
			_ = runDoctor(nil)
		case "q", "quit", "exit":
			return nil
		default:
			fmt.Fprintln(os.Stdout, "无效选择")
		}
		fmt.Fprint(os.Stdout, "按回车继续...")
		_, _ = reader.ReadString('\n')
	}
}
