// Package main 是 outlook-mcp MCP server 的入口。
//
// 当前阶段把已经完成的认证、账号和邮件能力暴露为 MCP tools：
//   - account_list：查看本地账号档案。
//   - auth_status：验证 token 缓存并尝试静默刷新。
//   - mail_list：读取指定账号的收件箱最新邮件。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kylineyes/outlook-manager/internal/auth"
	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

// version 表示当前构建版本。
// 默认值用于本地开发；正式发布时由 GitHub Actions 通过 -ldflags 注入 tag。
var version = "dev"

// emptyArgs 表示不接收参数的 MCP tool 输入。
type emptyArgs struct{}

// mailListArgs 是 mail_list tool 的输入参数。
type mailListArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少封邮件，默认 10，最大 100"`
}

// mailReadArgs 是 mail_read tool 的输入参数。
type mailReadArgs struct {
	ID      string `json:"id" jsonschema:"要读取的邮件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailSearchArgs 是 mail_search tool 的输入参数。
type mailSearchArgs struct {
	Query   string `json:"query" jsonschema:"搜索关键字"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少封邮件，默认 10，最大 100"`
}

// mailSendArgs 是 mail_send tool 的输入参数。
type mailSendArgs struct {
	To              []string `json:"to" jsonschema:"收件人邮箱列表"`
	Cc              []string `json:"cc,omitempty" jsonschema:"抄送邮箱列表"`
	Bcc             []string `json:"bcc,omitempty" jsonschema:"密送邮箱列表"`
	Subject         string   `json:"subject" jsonschema:"邮件主题"`
	Body            string   `json:"body" jsonschema:"邮件正文"`
	IsHTML          bool     `json:"isHtml,omitempty" jsonschema:"正文是否为 HTML，默认 false"`
	SaveToSentItems bool     `json:"saveToSentItems,omitempty" jsonschema:"是否保存到已发送邮件"`
	Account         string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailMarkArgs 是 mail_mark_read / mail_mark_unread tool 的输入参数。
type mailMarkArgs struct {
	ID      string `json:"id" jsonschema:"要操作的邮件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailFoldersArgs 是 mail_folders_list tool 的输入参数。
type mailFoldersArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少个文件夹，默认 50，最大 200"`
}

// mailFoldersResult 是 mail_folders_list tool 的返回结构。
type mailFoldersResult struct {
	Account string             `json:"account"`
	Folders []graph.MailFolder `json:"folders"`
}

// mailMoveArgs 是 mail_move tool 的输入参数。
type mailMoveArgs struct {
	ID      string `json:"id" jsonschema:"要移动的邮件 ID"`
	Folder  string `json:"folder" jsonschema:"目标文件夹名称或 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDeleteArgs 是 mail_delete tool 的输入参数。
type mailDeleteArgs struct {
	ID      string `json:"id" jsonschema:"要删除的邮件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailAttachmentsArgs 是 mail_attachments_list tool 的输入参数。
type mailAttachmentsArgs struct {
	ID      string `json:"id" jsonschema:"邮件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailAttachmentSaveArgs 是 mail_attachment_save tool 的输入参数。
type mailAttachmentSaveArgs struct {
	ID           string `json:"id" jsonschema:"邮件 ID"`
	AttachmentID string `json:"attachmentId" jsonschema:"附件 ID"`
	OutputPath   string `json:"outputPath" jsonschema:"保存到本地的完整路径"`
	Account      string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailReplyArgs 是 mail_reply / mail_reply_all tool 的输入参数。
type mailReplyArgs struct {
	ID      string `json:"id" jsonschema:"邮件 ID"`
	Body    string `json:"body" jsonschema:"回复正文"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailForwardArgs 是 mail_forward tool 的输入参数。
type mailForwardArgs struct {
	ID      string   `json:"id" jsonschema:"邮件 ID"`
	To      []string `json:"to" jsonschema:"转发收件人邮箱列表"`
	Body    string   `json:"body,omitempty" jsonschema:"转发附言"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDraftCreateArgs 是 mail_draft_create tool 的输入参数。
type mailDraftCreateArgs struct {
	To      []string `json:"to" jsonschema:"收件人邮箱列表"`
	Cc      []string `json:"cc,omitempty" jsonschema:"抄送邮箱列表"`
	Bcc     []string `json:"bcc,omitempty" jsonschema:"密送邮箱列表"`
	Subject string   `json:"subject" jsonschema:"邮件主题"`
	Body    string   `json:"body" jsonschema:"邮件正文"`
	IsHTML  bool     `json:"isHtml,omitempty" jsonschema:"正文是否为 HTML，默认 false"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDraftsArgs 是 mail_drafts_list tool 的输入参数。
type mailDraftsArgs struct {
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少封草稿，默认 10，最大 100"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDraftSendArgs 是 mail_draft_send tool 的输入参数。
type mailDraftSendArgs struct {
	ID      string `json:"id" jsonschema:"草稿 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailFolderCreateArgs 是 mail_folder_create tool 的输入参数。
type mailFolderCreateArgs struct {
	Name    string `json:"name" jsonschema:"文件夹名称"`
	Parent  string `json:"parent,omitempty" jsonschema:"父文件夹 ID；不填时创建在根目录"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailFolderRenameArgs 是 mail_folder_rename tool 的输入参数。
type mailFolderRenameArgs struct {
	ID      string `json:"id" jsonschema:"文件夹 ID"`
	Name    string `json:"name" jsonschema:"新的文件夹名称"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailFolderDeleteArgs 是 mail_folder_delete tool 的输入参数。
type mailFolderDeleteArgs struct {
	ID      string `json:"id" jsonschema:"文件夹 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailFlagArgs 是 mail_flag / mail_unflag tool 的输入参数。
type mailFlagArgs struct {
	ID      string `json:"id" jsonschema:"邮件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailImportanceArgs 是 mail_set_importance tool 的输入参数。
type mailImportanceArgs struct {
	ID      string `json:"id" jsonschema:"邮件 ID"`
	Level   string `json:"level" jsonschema:"重要性：low、normal 或 high"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// authStatusItem 是 auth_status tool 中单个账号的检查结果。
type authStatusItem struct {
	Username           string `json:"username"`
	HomeAccountID      string `json:"homeAccountId,omitempty"`
	TenantID           string `json:"tenantId,omitempty"`
	RefreshTokenCached bool   `json:"refreshTokenCached"`
	RefreshSuccess     bool   `json:"refreshSuccess"`
	ExpiresAt          string `json:"expiresAt,omitempty"`
	Error              string `json:"error,omitempty"`
}

// mailListResult 是 mail_list tool 的返回结构。
type mailListResult struct {
	Account  string          `json:"account"`
	Messages []graph.Message `json:"messages"`
}

// main 创建 MCP server、注册 tools，并通过 stdio 启动服务。
func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "outlook-mcp",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "account_list",
		Description: "列出本地已登录的 Outlook 账号档案，不包含 token",
	}, accountListHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "auth_status",
		Description: "检查本地 token 缓存，并尝试为每个账号静默刷新 access token",
	}, authStatusHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_list",
		Description: "读取指定账号或默认账号收件箱中的最新邮件",
	}, mailListHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_read",
		Description: "根据邮件 ID 读取单封邮件详情",
	}, mailReadHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_search",
		Description: "使用关键字搜索邮件",
	}, mailSearchHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_send",
		Description: "发送一封邮件",
	}, mailSendHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_folders_list",
		Description: "列出邮件文件夹",
	}, mailFoldersHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_folder_create",
		Description: "创建邮件文件夹",
	}, mailFolderCreateHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_folder_rename",
		Description: "重命名邮件文件夹",
	}, mailFolderRenameHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_folder_delete",
		Description: "删除邮件文件夹",
	}, mailFolderDeleteHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_flag",
		Description: "设置邮件旗标",
	}, mailFlagHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_unflag",
		Description: "取消邮件旗标",
	}, mailUnflagHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_set_importance",
		Description: "设置邮件重要性",
	}, mailImportanceHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_move",
		Description: "把邮件移动到指定文件夹",
	}, mailMoveHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_delete",
		Description: "删除邮件（移动到已删除邮件）",
	}, mailDeleteHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_attachments_list",
		Description: "列出邮件附件",
	}, mailAttachmentsHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_attachment_save",
		Description: "下载并保存邮件附件到本地文件",
	}, mailAttachmentSaveHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_reply",
		Description: "回复邮件",
	}, mailReplyHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_reply_all",
		Description: "回复全部收件人",
	}, mailReplyAllHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_forward",
		Description: "转发邮件",
	}, mailForwardHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_draft_create",
		Description: "创建草稿邮件",
	}, mailDraftCreateHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_drafts_list",
		Description: "列出草稿邮件",
	}, mailDraftsHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_draft_send",
		Description: "发送草稿邮件",
	}, mailDraftSendHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_mark_read",
		Description: "把指定邮件标记为已读",
	}, mailMarkReadHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_mark_unread",
		Description: "把指定邮件标记为未读",
	}, mailMarkUnreadHandler)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("outlook-mcp 运行失败: %v", err)
	}
}

// accountListHandler 返回 accounts.json 中的账号档案。
func accountListHandler(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	accounts, err := store.LoadAccounts()
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(accounts)
}

// authStatusHandler 检查所有本地账号的 token 是否可以正常刷新。
func authStatusHandler(ctx context.Context, req *mcp.CallToolRequest, args emptyArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}

	results := make([]authStatusItem, 0, len(accounts))
	for _, account := range accounts {
		item := authStatusItem{
			Username:           account.Username,
			HomeAccountID:      account.HomeAccountID,
			TenantID:           account.TenantID,
			RefreshTokenCached: account.RefreshTokenCached,
		}
		result, err := client.AcquireTokenSilent(ctx, account)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.RefreshSuccess = true
			item.ExpiresAt = result.Account.ExpiresAt.Format(time.RFC3339)
		}
		results = append(results, item)
	}
	return jsonResult(results)
}

// mailListHandler 读取指定账号的收件箱最新邮件。
func mailListHandler(ctx context.Context, req *mcp.CallToolRequest, args mailListArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}

	// 先静默刷新一次，确保 access token 可用，并验证 refresh token 是否仍然有效。
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	messages, err := graph.ListInboxMessages(ctx, loginResult.AccessToken, args.Top)
	if err != nil {
		return nil, nil, err
	}

	return jsonResult(mailListResult{
		Account:  selected.Username,
		Messages: messages,
	})
}

// mailReadHandler 根据邮件 ID 读取单封邮件详情。
func mailReadHandler(ctx context.Context, req *mcp.CallToolRequest, args mailReadArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	detail, err := graph.GetMessage(ctx, loginResult.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(detail)
}

// mailSearchHandler 使用关键字搜索邮件。
func mailSearchHandler(ctx context.Context, req *mcp.CallToolRequest, args mailSearchArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Query) == "" {
		return nil, nil, fmt.Errorf("搜索关键字不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	messages, err := graph.SearchMessages(ctx, loginResult.AccessToken, args.Query, args.Top)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(mailListResult{
		Account:  selected.Username,
		Messages: messages,
	})
}

// mailSendHandler 发送一封邮件。
func mailSendHandler(ctx context.Context, req *mcp.CallToolRequest, args mailSendArgs) (*mcp.CallToolResult, any, error) {
	if len(args.To) == 0 {
		return nil, nil, fmt.Errorf("至少需要一个收件人")
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}

	contentType := "Text"
	if args.IsHTML {
		contentType = "HTML"
	}
	input := graph.SendMailInput{
		To:              args.To,
		Cc:              args.Cc,
		Bcc:             args.Bcc,
		Subject:         args.Subject,
		Body:            args.Body,
		ContentType:     contentType,
		SaveToSentItems: args.SaveToSentItems,
	}
	if err := graph.SendMail(ctx, loginResult.AccessToken, input); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"to":      args.To,
		"subject": args.Subject,
	})
}

// mailMarkReadHandler 把邮件标记为已读。
func mailMarkReadHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMarkArgs) (*mcp.CallToolResult, any, error) {
	return markMessageRead(ctx, args, true)
}

// mailMarkUnreadHandler 把邮件标记为未读。
func mailMarkUnreadHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMarkArgs) (*mcp.CallToolResult, any, error) {
	return markMessageRead(ctx, args, false)
}

// markMessageRead 是 mail_mark_read / mail_mark_unread 的共用实现。
func markMessageRead(ctx context.Context, args mailMarkArgs, isRead bool) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.SetMessageRead(ctx, loginResult.AccessToken, args.ID, isRead); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"messageId": args.ID,
		"isRead":    isRead,
	})
}

// mailFoldersHandler 列出邮件文件夹。
func mailFoldersHandler(ctx context.Context, req *mcp.CallToolRequest, args mailFoldersArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	folders, err := graph.ListMailFolders(ctx, loginResult.AccessToken, args.Top)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(mailFoldersResult{
		Account: selected.Username,
		Folders: folders,
	})
}

// mailMoveHandler 把邮件移动到指定文件夹。
func mailMoveHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMoveArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}
	if strings.TrimSpace(args.Folder) == "" {
		return nil, nil, fmt.Errorf("目标文件夹不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	destinationID, err := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, args.Folder)
	if err != nil {
		return nil, nil, err
	}
	newMessageID, err := graph.MoveMessage(ctx, loginResult.AccessToken, args.ID, destinationID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":      true,
		"account":      selected.Username,
		"oldMessageId": args.ID,
		"newMessageId": newMessageID,
		"folder":       args.Folder,
	})
}

// mailDeleteHandler 删除邮件。
func mailDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.DeleteMessage(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"messageId": args.ID,
	})
}

// mailAttachmentsHandler 列出指定邮件的附件。
func mailAttachmentsHandler(ctx context.Context, req *mcp.CallToolRequest, args mailAttachmentsArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	attachments, err := graph.ListAttachments(ctx, loginResult.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account":     selected.Username,
		"attachments": attachments,
	})
}

// mailAttachmentSaveHandler 下载并保存指定附件。
func mailAttachmentSaveHandler(ctx context.Context, req *mcp.CallToolRequest, args mailAttachmentSaveArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}
	if strings.TrimSpace(args.AttachmentID) == "" {
		return nil, nil, fmt.Errorf("附件 ID 不能为空")
	}
	if strings.TrimSpace(args.OutputPath) == "" {
		return nil, nil, fmt.Errorf("输出路径不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	size, err := graph.SaveAttachment(ctx, loginResult.AccessToken, args.ID, args.AttachmentID, args.OutputPath)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":      true,
		"account":      selected.Username,
		"messageId":    args.ID,
		"attachmentId": args.AttachmentID,
		"outputPath":   args.OutputPath,
		"size":         size,
	})
}

// mailReplyHandler 回复邮件。
func mailReplyHandler(ctx context.Context, req *mcp.CallToolRequest, args mailReplyArgs) (*mcp.CallToolResult, any, error) {
	return replyMail(ctx, args, false)
}

// mailReplyAllHandler 回复全部收件人。
func mailReplyAllHandler(ctx context.Context, req *mcp.CallToolRequest, args mailReplyArgs) (*mcp.CallToolResult, any, error) {
	return replyMail(ctx, args, true)
}

// replyMail 是 mail_reply / mail_reply_all 的共用实现。
func replyMail(ctx context.Context, args mailReplyArgs, replyAll bool) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if replyAll {
		err = graph.ReplyAllMessage(ctx, loginResult.AccessToken, args.ID, args.Body)
	} else {
		err = graph.ReplyMessage(ctx, loginResult.AccessToken, args.ID, args.Body)
	}
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"messageId": args.ID,
		"replyAll":  replyAll,
	})
}

// mailForwardHandler 转发邮件。
func mailForwardHandler(ctx context.Context, req *mcp.CallToolRequest, args mailForwardArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}
	if len(args.To) == 0 {
		return nil, nil, fmt.Errorf("转发邮件至少需要一个收件人")
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.ForwardMessage(ctx, loginResult.AccessToken, args.ID, args.Body, args.To); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"messageId": args.ID,
		"to":        args.To,
	})
}

// mailDraftCreateHandler 创建草稿邮件。
func mailDraftCreateHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDraftCreateArgs) (*mcp.CallToolResult, any, error) {
	if len(args.To) == 0 {
		return nil, nil, fmt.Errorf("至少需要一个收件人")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	contentType := "Text"
	if args.IsHTML {
		contentType = "HTML"
	}
	draftID, err := graph.CreateDraft(ctx, loginResult.AccessToken, graph.SendMailInput{
		To:          args.To,
		Cc:          args.Cc,
		Bcc:         args.Bcc,
		Subject:     args.Subject,
		Body:        args.Body,
		ContentType: contentType,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"draftId": draftID,
	})
}

// mailDraftsHandler 列出草稿邮件。
func mailDraftsHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDraftsArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	drafts, err := graph.ListDrafts(ctx, loginResult.AccessToken, args.Top)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(mailListResult{
		Account:  selected.Username,
		Messages: drafts,
	})
}

// mailDraftSendHandler 发送草稿邮件。
func mailDraftSendHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDraftSendArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("草稿 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.SendDraft(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"draftId": args.ID,
	})
}

// mailFolderCreateHandler 创建邮件文件夹。
func mailFolderCreateHandler(ctx context.Context, req *mcp.CallToolRequest, args mailFolderCreateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Name) == "" {
		return nil, nil, fmt.Errorf("文件夹名称不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	folder, err := graph.CreateMailFolder(ctx, loginResult.AccessToken, args.Name, args.Parent)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"folder":  folder,
	})
}

// mailFolderRenameHandler 重命名邮件文件夹。
func mailFolderRenameHandler(ctx context.Context, req *mcp.CallToolRequest, args mailFolderRenameArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("文件夹 ID 不能为空")
	}
	if strings.TrimSpace(args.Name) == "" {
		return nil, nil, fmt.Errorf("文件夹名称不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.RenameMailFolder(ctx, loginResult.AccessToken, args.ID, args.Name); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"folderId": args.ID,
		"newName":  args.Name,
	})
}

// mailFolderDeleteHandler 删除邮件文件夹。
func mailFolderDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailFolderDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("文件夹 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.DeleteMailFolder(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"folderId": args.ID,
	})
}

// mailFlagHandler 设置邮件旗标。
func mailFlagHandler(ctx context.Context, req *mcp.CallToolRequest, args mailFlagArgs) (*mcp.CallToolResult, any, error) {
	return setMessageFlag(ctx, args, true)
}

// mailUnflagHandler 取消邮件旗标。
func mailUnflagHandler(ctx context.Context, req *mcp.CallToolRequest, args mailFlagArgs) (*mcp.CallToolResult, any, error) {
	return setMessageFlag(ctx, args, false)
}

// setMessageFlag 是 mail_flag / mail_unflag 的共用实现。
func setMessageFlag(ctx context.Context, args mailFlagArgs, flagged bool) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.SetMessageFlag(ctx, loginResult.AccessToken, args.ID, flagged); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"messageId": args.ID,
		"flagged":   flagged,
	})
}

// mailImportanceHandler 设置邮件重要性。
func mailImportanceHandler(ctx context.Context, req *mcp.CallToolRequest, args mailImportanceArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}
	if strings.TrimSpace(args.Level) == "" {
		return nil, nil, fmt.Errorf("重要性不能为空")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	selected, err := auth.SelectAccount(accounts, args.Account)
	if err != nil {
		return nil, nil, err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	if err := graph.SetMessageImportance(ctx, loginResult.AccessToken, args.ID, args.Level); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"messageId": args.ID,
		"level":     args.Level,
	})
}

// jsonResult 把任意结构序列化成缩进 JSON，并包装成 MCP 文本结果。
func jsonResult(value any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("序列化 MCP 工具结果失败: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}, nil, nil
}
