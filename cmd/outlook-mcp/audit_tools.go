package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kylineyes/outlook-manager/internal/audit"
)

type auditQueryArgs struct {
	Limit   int    `json:"limit,omitempty" jsonschema:"最多返回记录数"`
	Action  string `json:"action,omitempty" jsonschema:"action 前缀"`
	Account string `json:"account,omitempty" jsonschema:"账号过滤"`
	Query   string `json:"query,omitempty" jsonschema:"关键字过滤"`
	Since   string `json:"since,omitempty" jsonschema:"开始时间"`
	Until   string `json:"until,omitempty" jsonschema:"结束时间"`
}

type auditExportArgs struct {
	File    string `json:"file" jsonschema:"导出文件路径"`
	Format  string `json:"format" jsonschema:"csv 或 json"`
	Limit   int    `json:"limit,omitempty" jsonschema:"最多导出记录数"`
	Action  string `json:"action,omitempty" jsonschema:"action 前缀"`
	Account string `json:"account,omitempty" jsonschema:"账号过滤"`
	Query   string `json:"query,omitempty" jsonschema:"关键字过滤"`
	Since   string `json:"since,omitempty" jsonschema:"开始时间"`
	Until   string `json:"until,omitempty" jsonschema:"结束时间"`
}

type auditConfigSetArgs struct {
	MaxSizeMB     int64 `json:"maxSizeMB,omitempty" jsonschema:"单个日志大小上限 MB"`
	MaxBackups    int   `json:"maxBackups,omitempty" jsonschema:"历史文件数量"`
	RetentionDays int   `json:"retentionDays,omitempty" jsonschema:"保留天数"`
}

func registerAuditTools(server *mcp.Server) {
	addTool(server, newTool("audit_export", "导出审计日志为 CSV 或 JSON"), auditExportHandler)
	addTool(server, newTool("audit_stats", "统计审计日志"), auditStatsHandler)
	addTool(server, newTool("audit_verify", "校验审计日志哈希链"), auditVerifyHandler)
	addTool(server, newTool("audit_config_get", "读取审计日志配置"), auditConfigGetHandler)
	addTool(server, newTool("audit_config_set", "设置审计日志配置"), auditConfigSetHandler)
	addTool(server, newTool("audit_cleanup", "清理过期审计日志"), auditCleanupHandler)
}

func parseAuditToolTime(value string) (time.Time, error) {
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

func auditQueryFromArgs(args auditQueryArgs) (audit.QueryOptions, error) {
	since, err := parseAuditToolTime(args.Since)
	if err != nil {
		return audit.QueryOptions{}, err
	}
	until, err := parseAuditToolTime(args.Until)
	if err != nil {
		return audit.QueryOptions{}, err
	}
	return audit.QueryOptions{Limit: args.Limit, ActionPrefix: args.Action, Account: args.Account, Query: args.Query, Since: since, Until: until}, nil
}

func auditExportHandler(ctx context.Context, req *mcp.CallToolRequest, args auditExportArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("导出文件路径不能为空")
	}
	query, err := auditQueryFromArgs(auditQueryArgs{Limit: args.Limit, Action: args.Action, Account: args.Account, Query: args.Query, Since: args.Since, Until: args.Until})
	if err != nil {
		return nil, nil, err
	}
	entries, err := audit.Query(query)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Create(args.File)
	if err != nil {
		return nil, nil, fmt.Errorf("创建审计导出文件失败: %w", err)
	}
	defer file.Close()
	switch strings.ToLower(strings.TrimSpace(args.Format)) {
	case "csv":
		if err := audit.ExportCSV(file, entries); err != nil {
			return nil, nil, err
		}
	case "json":
		if err := audit.ExportJSON(file, entries); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("导出格式必须是 csv 或 json")
	}
	return jsonResult(map[string]any{"success": true, "file": args.File, "format": args.Format, "count": len(entries)})
}

func auditStatsHandler(ctx context.Context, req *mcp.CallToolRequest, args auditQueryArgs) (*mcp.CallToolResult, any, error) {
	query, err := auditQueryFromArgs(args)
	if err != nil {
		return nil, nil, err
	}
	entries, err := audit.Query(query)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "stats": audit.BuildStats(entries)})
}

func auditVerifyHandler(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	result, err := audit.Verify()
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "verify": result})
}

func auditConfigGetHandler(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	return jsonResult(map[string]any{"success": true, "settings": audit.LoadSettings()})
}

func auditConfigSetHandler(ctx context.Context, req *mcp.CallToolRequest, args auditConfigSetArgs) (*mcp.CallToolResult, any, error) {
	settings := audit.LoadSettings()
	if args.MaxSizeMB > 0 {
		settings.MaxSizeBytes = args.MaxSizeMB * 1024 * 1024
	}
	if args.MaxBackups > 0 {
		settings.MaxBackups = args.MaxBackups
	}
	if args.RetentionDays > 0 {
		settings.RetentionDays = args.RetentionDays
	}
	if err := audit.SaveSettings(settings); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "settings": settings})
}

func auditCleanupHandler(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	result, err := audit.Cleanup(audit.LoadSettings())
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "result": result})
}
