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
	Folder  string `json:"folder,omitempty" jsonschema:"文件夹名称或 ID；不填时使用收件箱"`
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
	Folder  string `json:"folder,omitempty" jsonschema:"文件夹名称或 ID；不填时搜索全部文件夹"`
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

// mailMessageCategoriesArgs 是 mail_message_categories tool 的输入参数。
type mailMessageCategoriesArgs struct {
	ID      string `json:"id" jsonschema:"邮件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailCategoryArgs 是 mail_category_add / mail_category_remove tool 的输入参数。
type mailCategoryArgs struct {
	ID       string `json:"id" jsonschema:"邮件 ID"`
	Category string `json:"category" jsonschema:"类别名称"`
	Account  string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarListArgs 是 calendar_list tool 的输入参数。
type calendarListArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Days    int    `json:"days,omitempty" jsonschema:"查询未来多少天，默认 7，最大 90"`
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少条事件，默认 50，最大 200"`
}

// contactsListArgs 是 contacts_list tool 的输入参数。
type contactsListArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Folder  string `json:"folder,omitempty" jsonschema:"联系人文件夹名称或 ID；不填时使用默认文件夹"`
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少个联系人，默认 50，最大 200"`
}

// calendarCreateArgs 是 calendar_create tool 的输入参数。
type calendarCreateArgs struct {
	Account         string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Subject         string   `json:"subject" jsonschema:"事件主题"`
	Start           string   `json:"start" jsonschema:"开始时间，RFC3339 或 2006-01-02 15:04:05"`
	End             string   `json:"end" jsonschema:"结束时间，RFC3339 或 2006-01-02 15:04:05"`
	Body            string   `json:"body,omitempty" jsonschema:"事件正文"`
	Location        string   `json:"location,omitempty" jsonschema:"事件地点"`
	TimeZone        string   `json:"timeZone,omitempty" jsonschema:"时区，默认 UTC"`
	IsAllDay        bool     `json:"isAllDay,omitempty" jsonschema:"是否全天事件"`
	Attendees       []string `json:"attendees,omitempty" jsonschema:"参与人邮箱列表"`
	ReminderMinutes int      `json:"reminderMinutes,omitempty" jsonschema:"提前多少分钟提醒"`
	RepeatType      string   `json:"repeatType,omitempty" jsonschema:"重复类型: daily/weekly/monthly/yearly"`
	RepeatInterval  int      `json:"repeatInterval,omitempty" jsonschema:"重复间隔"`
	RepeatCount     int      `json:"repeatCount,omitempty" jsonschema:"重复次数"`
	RepeatUntil     string   `json:"repeatUntil,omitempty" jsonschema:"重复结束日期 YYYY-MM-DD"`
}

// calendarUpdateArgs 是 calendar_update tool 的输入参数。
type calendarUpdateArgs struct {
	ID              string   `json:"id" jsonschema:"事件 ID"`
	Account         string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Subject         string   `json:"subject" jsonschema:"事件主题"`
	Start           string   `json:"start" jsonschema:"开始时间"`
	End             string   `json:"end" jsonschema:"结束时间"`
	Body            string   `json:"body,omitempty" jsonschema:"事件正文"`
	Location        string   `json:"location,omitempty" jsonschema:"事件地点"`
	TimeZone        string   `json:"timeZone,omitempty" jsonschema:"时区，默认 UTC"`
	IsAllDay        bool     `json:"isAllDay,omitempty" jsonschema:"是否全天事件"`
	Attendees       []string `json:"attendees,omitempty" jsonschema:"参与人邮箱列表"`
	ReminderMinutes int      `json:"reminderMinutes,omitempty" jsonschema:"提前多少分钟提醒"`
	RepeatType      string   `json:"repeatType,omitempty" jsonschema:"重复类型: daily/weekly/monthly/yearly"`
	RepeatInterval  int      `json:"repeatInterval,omitempty" jsonschema:"重复间隔"`
	RepeatCount     int      `json:"repeatCount,omitempty" jsonschema:"重复次数"`
	RepeatUntil     string   `json:"repeatUntil,omitempty" jsonschema:"重复结束日期 YYYY-MM-DD"`
}

// calendarDeleteArgs 是 calendar_delete tool 的输入参数。
type calendarDeleteArgs struct {
	ID      string `json:"id" jsonschema:"事件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsCreateArgs 是 contacts_create tool 的输入参数。
type contactsCreateArgs struct {
	Account        string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Folder         string   `json:"folder,omitempty" jsonschema:"联系人文件夹名称或 ID；不填时使用默认文件夹"`
	DisplayName    string   `json:"displayName" jsonschema:"显示名称"`
	GivenName      string   `json:"givenName,omitempty" jsonschema:"名"`
	Surname        string   `json:"surname,omitempty" jsonschema:"姓"`
	Emails         []string `json:"emails,omitempty" jsonschema:"邮箱列表"`
	CompanyName    string   `json:"companyName,omitempty" jsonschema:"公司"`
	JobTitle       string   `json:"jobTitle,omitempty" jsonschema:"职位"`
	MobilePhone    string   `json:"mobilePhone,omitempty" jsonschema:"手机号"`
	BusinessPhones []string `json:"businessPhones,omitempty" jsonschema:"工作电话列表"`
}

// contactsUpdateArgs 是 contacts_update tool 的输入参数。
type contactsUpdateArgs struct {
	ID             string   `json:"id" jsonschema:"联系人 ID"`
	Account        string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	DisplayName    string   `json:"displayName" jsonschema:"显示名称"`
	GivenName      string   `json:"givenName,omitempty" jsonschema:"名"`
	Surname        string   `json:"surname,omitempty" jsonschema:"姓"`
	Emails         []string `json:"emails,omitempty" jsonschema:"邮箱列表"`
	CompanyName    string   `json:"companyName,omitempty" jsonschema:"公司"`
	JobTitle       string   `json:"jobTitle,omitempty" jsonschema:"职位"`
	MobilePhone    string   `json:"mobilePhone,omitempty" jsonschema:"手机号"`
	BusinessPhones []string `json:"businessPhones,omitempty" jsonschema:"工作电话列表"`
}

// contactsDeleteArgs 是 contacts_delete tool 的输入参数。
type contactsDeleteArgs struct {
	ID      string `json:"id" jsonschema:"联系人 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsSearchArgs 是 contacts_search tool 的输入参数。
type contactsSearchArgs struct {
	Query   string `json:"query" jsonschema:"搜索关键字，按显示名称前缀匹配"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少个联系人，默认 20，最大 200"`
}

// contactsFoldersArgs 是 contacts_folders_list tool 的输入参数。
type contactsFoldersArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Top     int    `json:"top,omitempty" jsonschema:"最多返回多少个文件夹，默认 50，最大 200"`
}

// contactsFolderCreateArgs 是 contacts_folder_create tool 的输入参数。
type contactsFolderCreateArgs struct {
	Name    string `json:"name" jsonschema:"文件夹名称"`
	Parent  string `json:"parent,omitempty" jsonschema:"父文件夹 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsFolderDeleteArgs 是 contacts_folder_delete tool 的输入参数。
type contactsFolderDeleteArgs struct {
	ID      string `json:"id" jsonschema:"文件夹 ID"`
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
		Name:        "mail_message_categories",
		Description: "列出单封邮件的类别",
	}, mailMessageCategoriesHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_category_add",
		Description: "给邮件添加类别",
	}, mailCategoryAddHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail_category_remove",
		Description: "从邮件移除类别",
	}, mailCategoryRemoveHandler)

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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "calendar_list",
		Description: "读取未来指定天数内的日历事件",
	}, calendarListHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_list",
		Description: "读取联系人列表",
	}, contactsListHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "calendar_create",
		Description: "创建日历事件",
	}, calendarCreateHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "calendar_update",
		Description: "更新日历事件",
	}, calendarUpdateHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "calendar_delete",
		Description: "删除日历事件",
	}, calendarDeleteHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_create",
		Description: "创建联系人",
	}, contactsCreateHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_update",
		Description: "更新联系人",
	}, contactsUpdateHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_delete",
		Description: "删除联系人",
	}, contactsDeleteHandler)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_search",
		Description: "按显示名称前缀搜索联系人",
	}, contactsSearchHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_folders_list",
		Description: "列出联系人文件夹",
	}, contactsFoldersHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_folder_create",
		Description: "创建联系人文件夹",
	}, contactsFolderCreateHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contacts_folder_delete",
		Description: "删除联系人文件夹",
	}, contactsFolderDeleteHandler)
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
	var messages []graph.Message
	if strings.TrimSpace(args.Folder) == "" {
		messages, err = graph.ListInboxMessages(ctx, loginResult.AccessToken, args.Top)
	} else {
		folderID, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, args.Folder)
		if resolveErr != nil {
			return nil, nil, resolveErr
		}
		messages, err = graph.ListMessagesInFolder(ctx, loginResult.AccessToken, folderID, args.Top)
	}
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
	var messages []graph.Message
	if strings.TrimSpace(args.Folder) == "" {
		messages, err = graph.SearchMessages(ctx, loginResult.AccessToken, args.Query, args.Top)
	} else {
		folderID, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, args.Folder)
		if resolveErr != nil {
			return nil, nil, resolveErr
		}
		messages, err = graph.SearchMessagesInFolder(ctx, loginResult.AccessToken, folderID, args.Query, args.Top)
	}
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

// mailMessageCategoriesHandler 列出单封邮件的类别。
func mailMessageCategoriesHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMessageCategoriesArgs) (*mcp.CallToolResult, any, error) {
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
	categories, err := graph.GetMessageCategories(ctx, loginResult.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account":    selected.Username,
		"messageId":  args.ID,
		"categories": categories,
	})
}

// mailCategoryAddHandler 给邮件添加类别。
func mailCategoryAddHandler(ctx context.Context, req *mcp.CallToolRequest, args mailCategoryArgs) (*mcp.CallToolResult, any, error) {
	return changeMessageCategory(ctx, args, true)
}

// mailCategoryRemoveHandler 从邮件移除类别。
func mailCategoryRemoveHandler(ctx context.Context, req *mcp.CallToolRequest, args mailCategoryArgs) (*mcp.CallToolResult, any, error) {
	return changeMessageCategory(ctx, args, false)
}

// changeMessageCategory 是添加和移除邮件类别的共用实现。
func changeMessageCategory(ctx context.Context, args mailCategoryArgs, add bool) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 不能为空")
	}
	if strings.TrimSpace(args.Category) == "" {
		return nil, nil, fmt.Errorf("类别名称不能为空")
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
	var categories []string
	if add {
		categories, err = graph.AddMessageCategory(ctx, loginResult.AccessToken, args.ID, args.Category)
	} else {
		categories, err = graph.RemoveMessageCategory(ctx, loginResult.AccessToken, args.ID, args.Category)
	}
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":    true,
		"account":    selected.Username,
		"messageId":  args.ID,
		"categories": categories,
	})
}

// calendarListHandler 读取未来指定天数内的日历事件。
func calendarListHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarListArgs) (*mcp.CallToolResult, any, error) {
	days := args.Days
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
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

	start := time.Now().UTC()
	end := start.AddDate(0, 0, days)
	events, err := graph.ListCalendarEvents(ctx, loginResult.AccessToken, start, end, args.Top)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account": selected.Username,
		"events":  events,
	})
}

// contactsListHandler 读取联系人列表。
func contactsListHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsListArgs) (*mcp.CallToolResult, any, error) {
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
	var contacts []graph.Contact
	if strings.TrimSpace(args.Folder) == "" {
		contacts, err = graph.ListContacts(ctx, loginResult.AccessToken, args.Top)
	} else {
		folderID, resolveErr := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, args.Folder)
		if resolveErr != nil {
			return nil, nil, resolveErr
		}
		contacts, err = graph.ListContactsInFolder(ctx, loginResult.AccessToken, folderID, args.Top)
	}
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account":  selected.Username,
		"contacts": contacts,
	})
}

// parseCalendarTimeValue 解析 MCP 传入的日历时间。
func parseCalendarTimeValue(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", value)
}

// calendarCreateHandler 创建日历事件。
func calendarCreateHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarCreateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Subject) == "" || strings.TrimSpace(args.Start) == "" || strings.TrimSpace(args.End) == "" {
		return nil, nil, fmt.Errorf("主题、开始时间和结束时间不能为空")
	}
	start, err := parseCalendarTimeValue(args.Start)
	if err != nil {
		return nil, nil, err
	}
	end, err := parseCalendarTimeValue(args.End)
	if err != nil {
		return nil, nil, err
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
	event, err := graph.CreateCalendarEvent(ctx, loginResult.AccessToken, graph.CalendarEventInput{
		Subject:                    args.Subject,
		Body:                       args.Body,
		Start:                      start,
		End:                        end,
		TimeZone:                   args.TimeZone,
		Location:                   args.Location,
		IsAllDay:                   args.IsAllDay,
		Attendees:                  args.Attendees,
		ReminderMinutesBeforeStart: args.ReminderMinutes,
		RepeatType:                 args.RepeatType,
		RepeatInterval:             args.RepeatInterval,
		RepeatCount:                args.RepeatCount,
		RepeatUntil:                args.RepeatUntil,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"event":   event,
	})
}

// calendarUpdateHandler 更新日历事件。
func calendarUpdateHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarUpdateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("事件 ID 不能为空")
	}
	start, err := parseCalendarTimeValue(args.Start)
	if err != nil {
		return nil, nil, err
	}
	end, err := parseCalendarTimeValue(args.End)
	if err != nil {
		return nil, nil, err
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
	event, err := graph.UpdateCalendarEvent(ctx, loginResult.AccessToken, args.ID, graph.CalendarEventInput{
		Subject:                    args.Subject,
		Body:                       args.Body,
		Start:                      start,
		End:                        end,
		TimeZone:                   args.TimeZone,
		Location:                   args.Location,
		IsAllDay:                   args.IsAllDay,
		Attendees:                  args.Attendees,
		ReminderMinutesBeforeStart: args.ReminderMinutes,
		RepeatType:                 args.RepeatType,
		RepeatInterval:             args.RepeatInterval,
		RepeatCount:                args.RepeatCount,
		RepeatUntil:                args.RepeatUntil,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"event":   event,
	})
}

// calendarDeleteHandler 删除日历事件。
func calendarDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("事件 ID 不能为空")
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
	if err := graph.DeleteCalendarEvent(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"eventId": args.ID,
	})
}

// contactsCreateHandler 创建联系人。
func contactsCreateHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsCreateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.DisplayName) == "" {
		return nil, nil, fmt.Errorf("显示名称不能为空")
	}
	input := graph.ContactInput{
		DisplayName: args.DisplayName,
		GivenName:   args.GivenName,
		Surname:     args.Surname,
		CompanyName: args.CompanyName,
		JobTitle:    args.JobTitle,
		MobilePhone: args.MobilePhone,
	}
	for _, address := range args.Emails {
		input.EmailAddresses = append(input.EmailAddresses, graph.ContactEmailAddress{Address: address})
	}
	input.BusinessPhones = args.BusinessPhones

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
	var contact *graph.Contact
	if strings.TrimSpace(args.Folder) == "" {
		contact, err = graph.CreateContact(ctx, loginResult.AccessToken, input)
	} else {
		folderID, resolveErr := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, args.Folder)
		if resolveErr != nil {
			return nil, nil, resolveErr
		}
		contact, err = graph.CreateContactInFolder(ctx, loginResult.AccessToken, folderID, input)
	}
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"contact": contact,
	})
}

// contactsUpdateHandler 更新联系人。
func contactsUpdateHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsUpdateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.DisplayName) == "" {
		return nil, nil, fmt.Errorf("联系人 ID 和显示名称不能为空")
	}
	input := graph.ContactInput{
		DisplayName: args.DisplayName,
		GivenName:   args.GivenName,
		Surname:     args.Surname,
		CompanyName: args.CompanyName,
		JobTitle:    args.JobTitle,
		MobilePhone: args.MobilePhone,
	}
	for _, address := range args.Emails {
		input.EmailAddresses = append(input.EmailAddresses, graph.ContactEmailAddress{Address: address})
	}
	input.BusinessPhones = args.BusinessPhones

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
	contact, err := graph.UpdateContact(ctx, loginResult.AccessToken, args.ID, input)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"contact": contact,
	})
}

// contactsDeleteHandler 删除联系人。
func contactsDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("联系人 ID 不能为空")
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
	if err := graph.DeleteContact(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"contactId": args.ID,
	})
}

// contactsSearchHandler 按显示名称前缀搜索联系人。
func contactsSearchHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsSearchArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Query) == "" {
		return nil, nil, fmt.Errorf("搜索关键字不能为空")
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
	contacts, err := graph.SearchContacts(ctx, loginResult.AccessToken, args.Query, args.Top)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account":  selected.Username,
		"contacts": contacts,
	})
}

// contactsFoldersHandler 列出联系人文件夹。
func contactsFoldersHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsFoldersArgs) (*mcp.CallToolResult, any, error) {
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
	folders, err := graph.ListContactFolders(ctx, loginResult.AccessToken, args.Top)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account": selected.Username,
		"folders": folders,
	})
}

// contactsFolderCreateHandler 创建联系人文件夹。
func contactsFolderCreateHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsFolderCreateArgs) (*mcp.CallToolResult, any, error) {
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
	folder, err := graph.CreateContactFolder(ctx, loginResult.AccessToken, args.Name, args.Parent)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"folder":  folder,
	})
}

// contactsFolderDeleteHandler 删除联系人文件夹。
func contactsFolderDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsFolderDeleteArgs) (*mcp.CallToolResult, any, error) {
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
	if err := graph.DeleteContactFolder(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"folderId": args.ID,
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
