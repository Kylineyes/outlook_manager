package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/audit"
)

type auditQueryOptions struct {
	limit   int
	action  string
	account string
	query   string
	since   string
	until   string
	json    bool
	format  string
}

func addAuditQueryFlags(fs *flag.FlagSet, options *auditQueryOptions) {
	fs.IntVar(&options.limit, "limit", 50, "最多返回多少条记录")
	fs.StringVar(&options.action, "action", "", "按 action 前缀过滤")
	fs.StringVar(&options.account, "account", "", "按账号过滤")
	fs.StringVar(&options.query, "query", "", "在审计记录中搜索关键字")
	fs.StringVar(&options.since, "since", "", "开始时间，RFC3339 或 2006-01-02")
	fs.StringVar(&options.until, "until", "", "结束时间，RFC3339 或 2006-01-02")
	fs.BoolVar(&options.json, "json", false, "以 JSON 格式输出")
	fs.StringVar(&options.format, "format", "table", "输出格式: table/json/jsonl")
}

func parseAuditTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析审计时间: %s", value)
}

func (options auditQueryOptions) toQueryOptions() (audit.QueryOptions, error) {
	since, err := parseAuditTime(options.since)
	if err != nil {
		return audit.QueryOptions{}, err
	}
	until, err := parseAuditTime(options.until)
	if err != nil {
		return audit.QueryOptions{}, err
	}
	return audit.QueryOptions{
		Limit: options.limit, ActionPrefix: options.action, Account: options.account,
		Query: options.query, Since: since, Until: until,
	}, nil
}

func runAuditListQuery(args []string) error {
	fs := flag.NewFlagSet("audit list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options auditQueryOptions
	addAuditQueryFlags(fs, &options)
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	query, err := options.toQueryOptions()
	if err != nil {
		return err
	}
	entries, err := audit.Query(query)
	if err != nil {
		return err
	}
	return printAuditEntries(entries, options)
}

func printAuditEntries(entries []map[string]any, options auditQueryOptions) error {
	format := strings.ToLower(strings.TrimSpace(options.format))
	if options.json {
		format = "json"
	}
	switch format {
	case "json":
		return printJSON(entries)
	case "jsonl":
		for _, entry := range entries {
			data, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, string(data))
		}
		return nil
	default:
		for _, entry := range entries {
			timeText, _ := entry["time"].(string)
			action, _ := entry["action"].(string)
			account, _ := entry["account"].(string)
			data, _ := json.Marshal(entry)
			fmt.Fprintf(os.Stdout, "%s | %-24s | %-28s | %s\n", timeText, action, account, string(data))
		}
		return nil
	}
}

func runAuditExport(args []string) error {
	fs := flag.NewFlagSet("audit export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options auditQueryOptions
	addAuditQueryFlags(fs, &options)
	outFlag := fs.String("out", "", "导出文件路径")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*outFlag) == "" {
		return errors.New("用法: outlookctl audit export --out <文件> --format csv|json")
	}
	query, err := options.toQueryOptions()
	if err != nil {
		return err
	}
	entries, err := audit.Query(query)
	if err != nil {
		return err
	}
	file, err := os.Create(*outFlag)
	if err != nil {
		return fmt.Errorf("创建审计导出文件失败: %w", err)
	}
	defer file.Close()
	switch strings.ToLower(strings.TrimSpace(options.format)) {
	case "csv":
		if err := audit.ExportCSV(file, entries); err != nil {
			return err
		}
	case "json":
		if err := audit.ExportJSON(file, entries); err != nil {
			return err
		}
	default:
		return errors.New("导出格式必须是 csv 或 json")
	}
	fmt.Fprintf(os.Stdout, "已导出审计记录: %d\n", len(entries))
	fmt.Fprintf(os.Stdout, "文件: %s\n", *outFlag)
	return nil
}

func runAuditStats(args []string) error {
	fs := flag.NewFlagSet("audit stats", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options auditQueryOptions
	addAuditQueryFlags(fs, &options)
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	query, err := options.toQueryOptions()
	if err != nil {
		return err
	}
	entries, err := audit.Query(query)
	if err != nil {
		return err
	}
	stats := audit.BuildStats(entries)
	if options.json || strings.EqualFold(options.format, "json") {
		return printJSON(stats)
	}
	fmt.Fprintf(os.Stdout, "记录总数: %d\n", stats.Total)
	fmt.Fprintf(os.Stdout, "最早时间: %s\n", stats.First)
	fmt.Fprintf(os.Stdout, "最晚时间: %s\n", stats.Last)
	fmt.Fprintln(os.Stdout, "动作统计:")
	for action, count := range stats.Actions {
		fmt.Fprintf(os.Stdout, "  %s: %d\n", action, count)
	}
	fmt.Fprintln(os.Stdout, "账号统计:")
	for account, count := range stats.Accounts {
		fmt.Fprintf(os.Stdout, "  %s: %d\n", account, count)
	}
	return nil
}

func runAuditVerify(args []string) error {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	result, err := audit.Verify()
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "文件数量: %d\n", result.Files)
	fmt.Fprintf(os.Stdout, "校验记录: %d\n", result.Checked)
	fmt.Fprintf(os.Stdout, "旧格式记录: %d\n", result.Legacy)
	if result.Valid {
		fmt.Fprintln(os.Stdout, "校验结果: 通过")
		return nil
	}
	fmt.Fprintf(os.Stdout, "校验结果: 失败 (%s:%d) %s\n", result.BrokenFile, result.BrokenLine, result.Message)
	return errors.New("审计哈希链校验失败")
}

func runAuditConfig(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: outlookctl audit config <get|set>")
	}
	switch args[0] {
	case "get":
		settings := audit.LoadSettings()
		return printJSON(settings)
	case "set":
		fs := flag.NewFlagSet("audit config set", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		maxSizeMB := fs.Int64("max-size-mb", 10, "单个日志大小上限 MB")
		maxBackups := fs.Int("max-backups", 5, "历史文件数量")
		retentionDays := fs.Int("retention-days", 30, "保留天数")
		if err := parseCommandFlags(fs, args[1:]); err != nil {
			return err
		}
		settings := audit.Settings{MaxSizeBytes: *maxSizeMB * 1024 * 1024, MaxBackups: *maxBackups, RetentionDays: *retentionDays}
		if err := audit.SaveSettings(settings); err != nil {
			return err
		}
		return printJSON(settings)
	default:
		return fmt.Errorf("未知 audit config 子命令: %s", args[0])
	}
}

func runAuditCleanup(args []string) error {
	fs := flag.NewFlagSet("audit cleanup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	result, err := audit.Cleanup(audit.LoadSettings())
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "已清理审计文件: %d\n", result.Removed)
	return nil
}
