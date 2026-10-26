package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kylineyes/outlook-manager/internal/audit"
	"github.com/Kylineyes/outlook-manager/internal/eml"
	"github.com/Kylineyes/outlook-manager/internal/graph"
)

type mailBulkStatusArgs struct {
	IDs     []string `json:"ids" jsonschema:"邮件 ID 列表"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"确认实际执行批量操作"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

type mailBulkImportanceArgs struct {
	IDs     []string `json:"ids" jsonschema:"邮件 ID 列表"`
	Level   string   `json:"level" jsonschema:"重要性: low/normal/high"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"确认实际执行批量操作"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

type mailBulkCategoryArgs struct {
	IDs      []string `json:"ids" jsonschema:"邮件 ID 列表"`
	Category string   `json:"category" jsonschema:"类别名称"`
	Confirm  bool     `json:"confirm,omitempty" jsonschema:"确认实际执行批量操作"`
	Account  string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

type mailExportEMLArgs struct {
	ID      string `json:"id" jsonschema:"邮件 ID"`
	File    string `json:"file" jsonschema:"导出的 EML 文件路径"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

type mailImportEMLArgs struct {
	File    string `json:"file" jsonschema:"要导入的 EML 文件路径"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"确认创建草稿；不填时只 dry-run"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

func registerBulkStatusTools(server *mcp.Server) {
	addTool(server, newTool("mail_bulk_mark_read", "批量标记邮件为已读"), mailBulkMarkReadHandler)
	addTool(server, newTool("mail_bulk_mark_unread", "批量标记邮件为未读"), mailBulkMarkUnreadHandler)
	addTool(server, newTool("mail_bulk_flag", "批量设置邮件旗标"), mailBulkFlagHandler)
	addTool(server, newTool("mail_bulk_unflag", "批量取消邮件旗标"), mailBulkUnflagHandler)
	addTool(server, newTool("mail_bulk_set_importance", "批量设置邮件重要性"), mailBulkImportanceHandler)
	addTool(server, newTool("mail_bulk_category_add", "批量给邮件添加类别"), mailBulkCategoryAddHandler)
	addTool(server, newTool("mail_bulk_category_remove", "批量移除邮件类别"), mailBulkCategoryRemoveHandler)
}

func registerEMLTools(server *mcp.Server) {
	addTool(server, newTool("mail_export_eml", "导出邮件为 EML/MIME 文件"), mailExportEMLHandler)
	addTool(server, newTool("mail_import_eml", "从 EML 文件导入为草稿"), mailImportEMLHandler)
}

func mailBulkMarkReadHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkStatusArgs) (*mcp.CallToolResult, any, error) {
	return executeBulkRead(ctx, args, true)
}

func mailBulkMarkUnreadHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkStatusArgs) (*mcp.CallToolResult, any, error) {
	return executeBulkRead(ctx, args, false)
}

func executeBulkRead(ctx context.Context, args mailBulkStatusArgs, isRead bool) (*mcp.CallToolResult, any, error) {
	if len(args.IDs) == 0 {
		return nil, nil, fmt.Errorf("邮件 ID 列表不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	result, err := graph.BulkSetMessagesRead(ctx, login.AccessToken, args.IDs, isRead, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "result": result})
}

func mailBulkFlagHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkStatusArgs) (*mcp.CallToolResult, any, error) {
	return executeBulkFlag(ctx, args, true)
}

func mailBulkUnflagHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkStatusArgs) (*mcp.CallToolResult, any, error) {
	return executeBulkFlag(ctx, args, false)
}

func executeBulkFlag(ctx context.Context, args mailBulkStatusArgs, flagged bool) (*mcp.CallToolResult, any, error) {
	if len(args.IDs) == 0 {
		return nil, nil, fmt.Errorf("邮件 ID 列表不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	result, err := graph.BulkSetMessagesFlag(ctx, login.AccessToken, args.IDs, flagged, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "result": result})
}

func mailBulkImportanceHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkImportanceArgs) (*mcp.CallToolResult, any, error) {
	if len(args.IDs) == 0 || strings.TrimSpace(args.Level) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 列表和重要性不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	result, err := graph.BulkSetMessagesImportance(ctx, login.AccessToken, args.IDs, args.Level, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "result": result})
}

func mailBulkCategoryAddHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkCategoryArgs) (*mcp.CallToolResult, any, error) {
	return executeBulkCategory(ctx, args, true)
}

func mailBulkCategoryRemoveHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkCategoryArgs) (*mcp.CallToolResult, any, error) {
	return executeBulkCategory(ctx, args, false)
}

func executeBulkCategory(ctx context.Context, args mailBulkCategoryArgs, add bool) (*mcp.CallToolResult, any, error) {
	if len(args.IDs) == 0 || strings.TrimSpace(args.Category) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 列表和类别名称不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	var result *graph.BulkResult
	if add {
		result, err = graph.BulkAddMessagesCategory(ctx, login.AccessToken, args.IDs, args.Category, args.Confirm)
	} else {
		result, err = graph.BulkRemoveMessagesCategory(ctx, login.AccessToken, args.IDs, args.Category, args.Confirm)
	}
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "result": result})
}

func mailExportEMLHandler(ctx context.Context, req *mcp.CallToolRequest, args mailExportEMLArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 和导出文件路径不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	data, err := graph.GetMessageMIME(ctx, login.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	if dir := filepath.Dir(args.File); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, nil, fmt.Errorf("创建导出目录失败: %w", err)
		}
	}
	if err := os.WriteFile(args.File, data, 0o600); err != nil {
		return nil, nil, fmt.Errorf("写入 EML 文件失败: %w", err)
	}
	audit.Log("mail.eml.export", map[string]any{"messageId": args.ID, "size": len(data)})
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "file": args.File, "size": len(data)})
}

func mailImportEMLHandler(ctx context.Context, req *mcp.CallToolRequest, args mailImportEMLArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("EML 文件路径不能为空")
	}
	parsed, err := eml.ParseFile(args.File)
	if err != nil {
		return nil, nil, err
	}
	draft, err := eml.ToDraftInput(parsed)
	if err != nil {
		return nil, nil, err
	}
	defer draft.Cleanup()
	attachments := make([]map[string]any, 0, len(draft.Input.Attachments))
	for _, attachment := range draft.Input.Attachments {
		attachments = append(attachments, map[string]any{"name": attachment.Name, "size": attachment.Size})
	}
	if !args.Confirm {
		return jsonResult(map[string]any{
			"success": true, "dryRun": true, "subject": draft.Input.Subject,
			"to": draft.Input.To, "cc": draft.Input.Cc, "bcc": draft.Input.Bcc,
			"bodyType": draft.Input.ContentType, "attachments": attachments,
		})
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	draftID, err := graph.CreateDraft(ctx, login.AccessToken, draft.Input)
	if err != nil {
		return nil, nil, err
	}
	audit.Log("mail.eml.import", map[string]any{"draftId": draftID, "attachmentCount": len(draft.Input.Attachments)})
	return jsonResult(map[string]any{"success": true, "dryRun": false, "account": login.Account.Username, "draftId": draftID, "attachments": attachments})
}
