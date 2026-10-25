package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kylineyes/outlook-manager/internal/auth"
	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

// mailboxSettingsArgs 是 mailbox_settings_get tool 的输入参数。
type mailboxSettingsArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailboxAutoReplySetArgs 是 mailbox_auto_reply_set tool 的输入参数。
type mailboxAutoReplySetArgs struct {
	Status           string `json:"status" jsonschema:"状态: alwaysEnabled/disabled/scheduled"`
	InternalMessage  string `json:"internalMessage,omitempty" jsonschema:"组织内回复内容"`
	ExternalMessage  string `json:"externalMessage,omitempty" jsonschema:"组织外回复内容"`
	ExternalAudience string `json:"externalAudience,omitempty" jsonschema:"组织外收件人: none/contactsOnly/all"`
	Start            string `json:"start,omitempty" jsonschema:"开始时间，YYYY-MM-DD HH:mm:ss"`
	End              string `json:"end,omitempty" jsonschema:"结束时间，YYYY-MM-DD HH:mm:ss"`
	TimeZone         string `json:"timeZone,omitempty" jsonschema:"计划时间时区，默认 UTC"`
	Account          string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailboxTimezoneSetArgs 是 mailbox_timezone_set tool 的输入参数。
type mailboxTimezoneSetArgs struct {
	Value   string `json:"value" jsonschema:"IANA 或 Windows 时区"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailQueryArgs 是 mail_query tool 的输入参数。
type mailQueryArgs struct {
	Folder        string `json:"folder,omitempty" jsonschema:"文件夹名称或 ID；all 表示全邮箱，默认 inbox"`
	From          string `json:"from,omitempty" jsonschema:"发件人邮箱或关键字"`
	To            string `json:"to,omitempty" jsonschema:"收件人邮箱或姓名关键字"`
	Subject       string `json:"subject,omitempty" jsonschema:"主题关键字"`
	Body          string `json:"body,omitempty" jsonschema:"正文预览关键字"`
	Search        string `json:"search,omitempty" jsonschema:"主题加正文关键字"`
	Importance    string `json:"importance,omitempty" jsonschema:"重要性: low/normal/high"`
	Category      string `json:"category,omitempty" jsonschema:"类别名称"`
	After         string `json:"after,omitempty" jsonschema:"开始时间，YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss"`
	Before        string `json:"before,omitempty" jsonschema:"结束时间，YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss"`
	UnreadOnly    bool   `json:"unreadOnly,omitempty" jsonschema:"仅未读邮件"`
	HasAttachment bool   `json:"hasAttachment,omitempty" jsonschema:"仅带附件邮件"`
	Top           int    `json:"top,omitempty" jsonschema:"最多返回多少封邮件，默认 100，最大 1000"`
	Account       string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailViewSaveArgs 是 mail_view_save tool 的输入参数。
type mailViewSaveArgs struct {
	Name          string `json:"name" jsonschema:"视图名称"`
	Folder        string `json:"folder,omitempty" jsonschema:"文件夹名称或 ID"`
	From          string `json:"from,omitempty" jsonschema:"发件人"`
	To            string `json:"to,omitempty" jsonschema:"收件人"`
	Subject       string `json:"subject,omitempty" jsonschema:"主题关键字"`
	Body          string `json:"body,omitempty" jsonschema:"正文关键字"`
	Search        string `json:"search,omitempty" jsonschema:"主题加正文关键字"`
	Importance    string `json:"importance,omitempty" jsonschema:"重要性"`
	Category      string `json:"category,omitempty" jsonschema:"类别"`
	After         string `json:"after,omitempty" jsonschema:"开始时间"`
	Before        string `json:"before,omitempty" jsonschema:"结束时间"`
	UnreadOnly    bool   `json:"unreadOnly,omitempty" jsonschema:"仅未读"`
	HasAttachment bool   `json:"hasAttachment,omitempty" jsonschema:"仅带附件"`
	Top           int    `json:"top,omitempty" jsonschema:"最多返回数量"`
}

// mailViewRunArgs 是 mail_view_run tool 的输入参数。
type mailViewRunArgs struct {
	Name    string `json:"name" jsonschema:"视图名称"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailViewDeleteArgs 是 mail_view_delete tool 的输入参数。
type mailViewDeleteArgs struct {
	Name string `json:"name" jsonschema:"视图名称"`
}

func registerMailboxTools(server *mcp.Server) {
	addTool(server, newTool("mailbox_settings_get", "读取邮箱设置"), mailboxSettingsGetHandler)
	addTool(server, newTool("mailbox_auto_reply_get", "读取自动回复设置"), mailboxAutoReplyGetHandler)
	addTool(server, newTool("mailbox_auto_reply_set", "设置自动回复"), mailboxAutoReplySetHandler)
	addTool(server, newTool("mailbox_timezone_get", "读取邮箱时区"), mailboxTimezoneGetHandler)
	addTool(server, newTool("mailbox_timezone_set", "设置邮箱时区"), mailboxTimezoneSetHandler)
}

func registerMailQueryTools(server *mcp.Server) {
	addTool(server, newTool("mail_query", "高级邮件查询"), mailQueryHandler)
	addTool(server, newTool("mail_view_save", "保存邮件视图"), mailViewSaveHandler)
	addTool(server, newTool("mail_view_list", "列出邮件视图"), mailViewListHandler)
	addTool(server, newTool("mail_view_run", "运行邮件视图"), mailViewRunHandler)
	addTool(server, newTool("mail_view_delete", "删除邮件视图"), mailViewDeleteHandler)
}

func mailboxLogin(ctx context.Context, account string) (*auth.LoginResult, error) {
	client, err := auth.NewClient()
	if err != nil {
		return nil, err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return nil, err
	}
	return client.AcquireTokenSilent(ctx, selected)
}

func mailboxSettingsGetHandler(ctx context.Context, req *mcp.CallToolRequest, args mailboxSettingsArgs) (*mcp.CallToolResult, any, error) {
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	settings, err := graph.GetMailboxSettings(ctx, login.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "settings": settings})
}

func mailboxAutoReplyGetHandler(ctx context.Context, req *mcp.CallToolRequest, args mailboxSettingsArgs) (*mcp.CallToolResult, any, error) {
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	settings, err := graph.GetMailboxSettings(ctx, login.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "autoReply": settings.AutomaticRepliesSetting})
}

func mailboxAutoReplySetHandler(ctx context.Context, req *mcp.CallToolRequest, args mailboxAutoReplySetArgs) (*mcp.CallToolResult, any, error) {
	setting := graph.AutomaticRepliesSetting{
		Status:               args.Status,
		ExternalAudience:     args.ExternalAudience,
		InternalReplyMessage: args.InternalMessage,
		ExternalReplyMessage: args.ExternalMessage,
	}
	if strings.EqualFold(args.Status, "scheduled") {
		if strings.TrimSpace(args.Start) == "" || strings.TrimSpace(args.End) == "" {
			return nil, nil, fmt.Errorf("scheduled 状态需要 start 和 end")
		}
		start, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(args.Start))
		if err != nil {
			return nil, nil, fmt.Errorf("解析开始时间失败: %w", err)
		}
		end, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(args.End))
		if err != nil {
			return nil, nil, fmt.Errorf("解析结束时间失败: %w", err)
		}
		timeZone := args.TimeZone
		if strings.TrimSpace(timeZone) == "" {
			timeZone = "UTC"
		}
		setting.ScheduledStartDateTime = &graph.DateTimeTimeZone{DateTime: start.Format("2006-01-02T15:04:05"), TimeZone: timeZone}
		setting.ScheduledEndDateTime = &graph.DateTimeTimeZone{DateTime: end.Format("2006-01-02T15:04:05"), TimeZone: timeZone}
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	settings, err := graph.SetAutomaticReplies(ctx, login.AccessToken, setting)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "autoReply": settings.AutomaticRepliesSetting})
}

func mailboxTimezoneGetHandler(ctx context.Context, req *mcp.CallToolRequest, args mailboxSettingsArgs) (*mcp.CallToolResult, any, error) {
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	settings, err := graph.GetMailboxSettings(ctx, login.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "timeZone": settings.TimeZone})
}

func mailboxTimezoneSetHandler(ctx context.Context, req *mcp.CallToolRequest, args mailboxTimezoneSetArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Value) == "" {
		return nil, nil, fmt.Errorf("时区不能为空")
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	settings, err := graph.SetMailboxTimeZone(ctx, login.AccessToken, args.Value)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "timeZone": settings.TimeZone})
}

func normalizeMCPQueryTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("无法解析时间: %s", value)
}

func (args mailQueryArgs) toGraphQuery() (graph.MessageQuery, error) {
	after, err := normalizeMCPQueryTime(args.After)
	if err != nil {
		return graph.MessageQuery{}, err
	}
	before, err := normalizeMCPQueryTime(args.Before)
	if err != nil {
		return graph.MessageQuery{}, err
	}
	folder := strings.TrimSpace(args.Folder)
	if folder == "" {
		folder = "inbox"
	}
	if strings.EqualFold(folder, "all") {
		folder = ""
	}
	return graph.MessageQuery{
		Folder: folder, From: args.From, To: args.To, Subject: args.Subject,
		Body: args.Body, Search: args.Search, Importance: args.Importance,
		Category: args.Category, After: after, Before: before,
		UnreadOnly: args.UnreadOnly, HasAttachment: args.HasAttachment, Top: args.Top,
	}, nil
}

func mailQueryHandler(ctx context.Context, req *mcp.CallToolRequest, args mailQueryArgs) (*mcp.CallToolResult, any, error) {
	query, err := args.toGraphQuery()
	if err != nil {
		return nil, nil, err
	}
	login, err := mailboxLogin(ctx, args.Account)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(query.Folder) != "" {
		folderID, err := graph.ResolveMailFolderID(ctx, login.AccessToken, query.Folder)
		if err != nil {
			return nil, nil, err
		}
		query.Folder = folderID
	}
	messages, err := graph.QueryMessages(ctx, login.AccessToken, query)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "messages": messages, "count": len(messages)})
}

func mailViewSaveHandler(ctx context.Context, req *mcp.CallToolRequest, args mailViewSaveArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Name) == "" {
		return nil, nil, fmt.Errorf("视图名称不能为空")
	}
	view := store.MailView{
		Name: args.Name, Folder: args.Folder, From: args.From, To: args.To,
		Subject: args.Subject, Body: args.Body, Search: args.Search,
		Importance: args.Importance, Category: args.Category, After: args.After,
		Before: args.Before, UnreadOnly: args.UnreadOnly, HasAttachment: args.HasAttachment,
		Top: args.Top,
	}
	file, err := store.UpsertMailView(view)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "name": args.Name, "views": file.Views})
}

func mailViewListHandler(ctx context.Context, req *mcp.CallToolRequest, args mailboxSettingsArgs) (*mcp.CallToolResult, any, error) {
	file, err := store.LoadMailViews()
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "views": file.Views, "count": len(file.Views)})
}

func mailViewRunHandler(ctx context.Context, req *mcp.CallToolRequest, args mailViewRunArgs) (*mcp.CallToolResult, any, error) {
	file, err := store.LoadMailViews()
	if err != nil {
		return nil, nil, err
	}
	for _, view := range file.Views {
		if !strings.EqualFold(view.Name, args.Name) {
			continue
		}
		queryArgs := mailQueryArgs{
			Folder: view.Folder, From: view.From, To: view.To, Subject: view.Subject,
			Body: view.Body, Search: view.Search, Importance: view.Importance,
			Category: view.Category, After: view.After, Before: view.Before,
			UnreadOnly: view.UnreadOnly, HasAttachment: view.HasAttachment, Top: view.Top,
			Account: args.Account,
		}
		query, err := queryArgs.toGraphQuery()
		if err != nil {
			return nil, nil, err
		}
		login, err := mailboxLogin(ctx, args.Account)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(query.Folder) != "" {
			folderID, err := graph.ResolveMailFolderID(ctx, login.AccessToken, query.Folder)
			if err != nil {
				return nil, nil, err
			}
			query.Folder = folderID
		}
		messages, err := graph.QueryMessages(ctx, login.AccessToken, query)
		if err != nil {
			return nil, nil, err
		}
		return jsonResult(map[string]any{"success": true, "account": login.Account.Username, "view": view, "messages": messages, "count": len(messages)})
	}
	return nil, nil, fmt.Errorf("找不到邮件视图: %s", args.Name)
}

func mailViewDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailViewDeleteArgs) (*mcp.CallToolResult, any, error) {
	file, err := store.DeleteMailView(args.Name)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "name": args.Name, "views": file.Views})
}
