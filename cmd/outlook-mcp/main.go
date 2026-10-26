// Package main 是 outlook-mcp MCP server 的入口。
//
// 当前阶段把已经完成的认证、账号和邮件能力暴露为 MCP tools：
//   - account_list：查看本地账号档案。
//   - auth_status：验证 token 缓存并尝试静默刷新。
//   - mail_list：读取指定账号的收件箱最新邮件。
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kylineyes/outlook-manager/internal/audit"
	"github.com/Kylineyes/outlook-manager/internal/auth"
	"github.com/Kylineyes/outlook-manager/internal/contactcsv"
	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
	"github.com/Kylineyes/outlook-manager/internal/vcard"
)

// newTool 创建带 MCP annotations 的 tool 定义。
func newTool(name, description string) *mcp.Tool {
	readOnly := isReadOnlyTool(name)
	destructive := isDestructiveTool(name)
	openWorld := name != "account_list" && !strings.HasPrefix(name, "audit_")
	return &mcp.Tool{
		Name:        name,
		Title:       description,
		Description: description,
		OutputSchema: map[string]any{
			"$id":                  "https://outlook-manager.local/schemas/tools/" + name + ".json",
			"title":                description,
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"success": map[string]any{"type": "boolean"},
				"account": map[string]any{"type": "string"},
				"result":  map[string]any{"type": "object", "additionalProperties": true},
				"error": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code":    map[string]any{"type": "string"},
						"message": map[string]any{"type": "string"},
					},
					"required":             []string{"code", "message"},
					"additionalProperties": false,
				},
			},
			"required": []string{"result"},
		},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readOnly,
			DestructiveHint: &destructive,
			OpenWorldHint:   &openWorld,
			IdempotentHint:  readOnly,
		},
	}
}

// addTool 注册 tool，并把 handler 错误转换为统一的 MCP structured error。
func addTool[In any](server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, any]) {
	wrapped := func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		result, output, err := handler(ctx, req, input)
		if err != nil {
			return toolErrorResult(err), nil, nil
		}
		return result, output, nil
	}
	mcp.AddTool(server, tool, wrapped)
}

func isReadOnlyTool(name string) bool {
	switch name {
	case "account_list", "audit_list", "audit_stats", "audit_verify", "audit_config_get", "calendar_series_info", "calendar_free_busy", "calendar_find_meeting_times", "contacts_duplicates_scan", "sync_status", "local_search", "mailbox_settings_get", "mailbox_auto_reply_get", "mailbox_timezone_get", "mail_query", "mail_view_list", "mail_folder_tree", "mail_list", "mail_read", "mail_search", "mail_folders_list",
		"mail_attachments_list", "mail_message_categories", "mail_rules_list",
		"mail_master_categories_list", "calendar_list", "calendar_calendars_list",
		"calendar_attachments_list", "calendar_attachment_get", "contacts_list",
		"contacts_search", "contacts_folders_list", "contacts_photo_get":
		return true
	default:
		return false
	}
}

func isDestructiveTool(name string) bool {
	switch name {
	case "mail_delete", "mail_bulk_delete", "mail_permanent_delete", "mail_folder_delete", "mail_view_delete", "calendar_series_delete", "contacts_duplicates_merge", "sync_reset", "audit_cleanup",
		"mail_rule_delete", "mail_rule_apply", "mail_master_category_delete",
		"mail_draft_attachment_delete", "mail_draft_attachment_replace",
		"calendar_delete", "calendar_cancel", "calendar_attachment_delete", "calendar_attachment_replace", "calendar_attachment_bulk_delete", "contacts_delete", "contacts_bulk_delete", "mail_draft_attachment_bulk_delete",
		"contacts_permanent_delete", "contacts_folder_delete":
		return true
	default:
		return false
	}
}

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
	All     bool   `json:"all,omitempty" jsonschema:"是否分页拉取多页邮件"`
	Max     int    `json:"max,omitempty" jsonschema:"全量拉取时的最大邮件数量，默认 500，最大 1000"`
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
	To                     []string `json:"to" jsonschema:"收件人邮箱列表"`
	Cc                     []string `json:"cc,omitempty" jsonschema:"抄送邮箱列表"`
	Bcc                    []string `json:"bcc,omitempty" jsonschema:"密送邮箱列表"`
	Subject                string   `json:"subject" jsonschema:"邮件主题"`
	Body                   string   `json:"body" jsonschema:"邮件正文"`
	IsHTML                 bool     `json:"isHtml,omitempty" jsonschema:"正文是否为 HTML，默认 false"`
	SaveToSentItems        bool     `json:"saveToSentItems,omitempty" jsonschema:"是否保存到已发送邮件"`
	Account                string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Attachments            []string `json:"attachments,omitempty" jsonschema:"本地附件文件路径列表"`
	Importance             string   `json:"importance,omitempty" jsonschema:"重要性: low/normal/high"`
	RequestReadReceipt     bool     `json:"requestReadReceipt,omitempty" jsonschema:"请求已读回执"`
	RequestDeliveryReceipt bool     `json:"requestDeliveryReceipt,omitempty" jsonschema:"请求送达回执"`
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
	To                     []string `json:"to" jsonschema:"收件人邮箱列表"`
	Cc                     []string `json:"cc,omitempty" jsonschema:"抄送邮箱列表"`
	Bcc                    []string `json:"bcc,omitempty" jsonschema:"密送邮箱列表"`
	Subject                string   `json:"subject" jsonschema:"邮件主题"`
	Body                   string   `json:"body" jsonschema:"邮件正文"`
	IsHTML                 bool     `json:"isHtml,omitempty" jsonschema:"正文是否为 HTML，默认 false"`
	Account                string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Attachments            []string `json:"attachments,omitempty" jsonschema:"本地附件文件路径列表"`
	Importance             string   `json:"importance,omitempty" jsonschema:"重要性: low/normal/high"`
	RequestReadReceipt     bool     `json:"requestReadReceipt,omitempty" jsonschema:"请求已读回执"`
	RequestDeliveryReceipt bool     `json:"requestDeliveryReceipt,omitempty" jsonschema:"请求送达回执"`
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
	Account  string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Days     int    `json:"days,omitempty" jsonschema:"查询未来多少天，默认 7，最大 90"`
	Top      int    `json:"top,omitempty" jsonschema:"最多返回多少条事件，默认 50，最大 200"`
	Calendar string `json:"calendar,omitempty" jsonschema:"日历名称或 ID；不填时使用默认日历"`
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
	Categories      []string `json:"categories,omitempty" jsonschema:"事件类别列表"`
	ShowAs          string   `json:"showAs,omitempty" jsonschema:"日程状态: free/tentative/busy/oof/workingElsewhere/unknown"`
	Importance      string   `json:"importance,omitempty" jsonschema:"事件重要性: low/normal/high"`
	Sensitivity     string   `json:"sensitivity,omitempty" jsonschema:"事件敏感度: normal/personal/private/confidential"`
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
	Categories      []string `json:"categories,omitempty" jsonschema:"事件类别列表"`
	ShowAs          string   `json:"showAs,omitempty" jsonschema:"日程状态: free/tentative/busy/oof/workingElsewhere/unknown"`
	Importance      string   `json:"importance,omitempty" jsonschema:"事件重要性: low/normal/high"`
	Sensitivity     string   `json:"sensitivity,omitempty" jsonschema:"事件敏感度: normal/personal/private/confidential"`
}

// calendarDeleteArgs 是 calendar_delete tool 的输入参数。
type calendarDeleteArgs struct {
	ID      string `json:"id" jsonschema:"事件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsCreateArgs 是 contacts_create tool 的输入参数。
type contactsCreateArgs struct {
	Account          string                 `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Folder           string                 `json:"folder,omitempty" jsonschema:"联系人文件夹名称或 ID；不填时使用默认文件夹"`
	DisplayName      string                 `json:"displayName" jsonschema:"显示名称"`
	GivenName        string                 `json:"givenName,omitempty" jsonschema:"名"`
	Surname          string                 `json:"surname,omitempty" jsonschema:"姓"`
	Emails           []string               `json:"emails,omitempty" jsonschema:"邮箱列表"`
	CompanyName      string                 `json:"companyName,omitempty" jsonschema:"公司"`
	JobTitle         string                 `json:"jobTitle,omitempty" jsonschema:"职位"`
	MobilePhone      string                 `json:"mobilePhone,omitempty" jsonschema:"手机号"`
	BusinessPhones   []string               `json:"businessPhones,omitempty" jsonschema:"工作电话列表"`
	NickName         string                 `json:"nickName,omitempty" jsonschema:"昵称"`
	Birthday         string                 `json:"birthday,omitempty" jsonschema:"生日，格式 YYYY-MM-DD"`
	PersonalNotes    string                 `json:"personalNotes,omitempty" jsonschema:"个人备注"`
	Department       string                 `json:"department,omitempty" jsonschema:"部门"`
	OfficeLocation   string                 `json:"officeLocation,omitempty" jsonschema:"办公室位置"`
	Profession       string                 `json:"profession,omitempty" jsonschema:"职业"`
	BusinessHomePage string                 `json:"businessHomePage,omitempty" jsonschema:"业务主页网址"`
	HomeAddress      *graph.PhysicalAddress `json:"homeAddress,omitempty" jsonschema:"家庭地址"`
	BusinessAddress  *graph.PhysicalAddress `json:"businessAddress,omitempty" jsonschema:"工作地址"`
	OtherAddress     *graph.PhysicalAddress `json:"otherAddress,omitempty" jsonschema:"其他地址"`
}

// contactsUpdateArgs 是 contacts_update tool 的输入参数。
type contactsUpdateArgs struct {
	ID               string                 `json:"id" jsonschema:"联系人 ID"`
	Account          string                 `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	DisplayName      string                 `json:"displayName" jsonschema:"显示名称"`
	GivenName        string                 `json:"givenName,omitempty" jsonschema:"名"`
	Surname          string                 `json:"surname,omitempty" jsonschema:"姓"`
	Emails           []string               `json:"emails,omitempty" jsonschema:"邮箱列表"`
	CompanyName      string                 `json:"companyName,omitempty" jsonschema:"公司"`
	JobTitle         string                 `json:"jobTitle,omitempty" jsonschema:"职位"`
	MobilePhone      string                 `json:"mobilePhone,omitempty" jsonschema:"手机号"`
	BusinessPhones   []string               `json:"businessPhones,omitempty" jsonschema:"工作电话列表"`
	NickName         string                 `json:"nickName,omitempty" jsonschema:"昵称"`
	Birthday         string                 `json:"birthday,omitempty" jsonschema:"生日，格式 YYYY-MM-DD"`
	PersonalNotes    string                 `json:"personalNotes,omitempty" jsonschema:"个人备注"`
	Department       string                 `json:"department,omitempty" jsonschema:"部门"`
	OfficeLocation   string                 `json:"officeLocation,omitempty" jsonschema:"办公室位置"`
	Profession       string                 `json:"profession,omitempty" jsonschema:"职业"`
	BusinessHomePage string                 `json:"businessHomePage,omitempty" jsonschema:"业务主页网址"`
	HomeAddress      *graph.PhysicalAddress `json:"homeAddress,omitempty" jsonschema:"家庭地址"`
	BusinessAddress  *graph.PhysicalAddress `json:"businessAddress,omitempty" jsonschema:"工作地址"`
	OtherAddress     *graph.PhysicalAddress `json:"otherAddress,omitempty" jsonschema:"其他地址"`
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

// mailRulesArgs 是 mail_rules_list tool 的输入参数。
type mailRulesArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailRuleCreateArgs 是 mail_rule_create tool 的输入参数。
type mailRuleCreateArgs struct {
	Account               string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Name                  string   `json:"name" jsonschema:"规则名称"`
	From                  []string `json:"from,omitempty" jsonschema:"发件人邮箱列表"`
	SubjectContains       []string `json:"subjectContains,omitempty" jsonschema:"主题包含关键字列表"`
	MoveTo                string   `json:"moveTo,omitempty" jsonschema:"移动到文件夹名称或 ID"`
	MarkAsRead            bool     `json:"markAsRead,omitempty" jsonschema:"标记为已读"`
	Delete                bool     `json:"delete,omitempty" jsonschema:"删除邮件"`
	Stop                  bool     `json:"stop,omitempty" jsonschema:"停止处理后续规则"`
	Disabled              bool     `json:"disabled,omitempty" jsonschema:"创建为禁用状态"`
	ApplyNow              bool     `json:"applyNow,omitempty" jsonschema:"创建后立即回放规则"`
	ConfirmApply          bool     `json:"confirmApply,omitempty" jsonschema:"确认实际修改已有邮件；不填时只 dry-run"`
	ApplyFolder           string   `json:"applyFolder,omitempty" jsonschema:"回放规则的目标文件夹；不填时使用收件箱"`
	ApplyMax              int      `json:"applyMax,omitempty" jsonschema:"回放时最多扫描多少封邮件，默认 500"`
	AssignCategories      []string `json:"assignCategories,omitempty" jsonschema:"给邮件添加的类别列表"`
	ApplyFolders          []string `json:"applyFolders,omitempty" jsonschema:"回放规则的目标文件夹列表"`
	BodyContains          []string `json:"bodyContains,omitempty" jsonschema:"正文包含关键字列表"`
	BodyOrSubjectContains []string `json:"bodyOrSubjectContains,omitempty" jsonschema:"正文或主题包含关键字列表"`
	HasAttachments        bool     `json:"hasAttachments,omitempty" jsonschema:"匹配带附件的邮件"`
	Importance            string   `json:"importance,omitempty" jsonschema:"匹配邮件重要性: low/normal/high"`
	SenderContains        []string `json:"senderContains,omitempty" jsonschema:"发件人包含关键字列表"`
	RecipientContains     []string `json:"recipientContains,omitempty" jsonschema:"收件人包含关键字列表"`
	CopyTo                string   `json:"copyTo,omitempty" jsonschema:"复制到文件夹名称或 ID"`
	ForwardTo             []string `json:"forwardTo,omitempty" jsonschema:"转发到邮箱列表"`
	RedirectTo            []string `json:"redirectTo,omitempty" jsonschema:"重定向到邮箱列表"`
	MarkImportance        string   `json:"markImportance,omitempty" jsonschema:"设置邮件重要性: low/normal/high"`
	PermanentDelete       bool     `json:"permanentDelete,omitempty" jsonschema:"永久删除邮件"`
}

// mailRuleUpdateArgs 是 mail_rule_update tool 的输入参数。
type mailRuleUpdateArgs struct {
	Account               string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	ID                    string   `json:"id" jsonschema:"规则 ID"`
	Name                  string   `json:"name" jsonschema:"规则名称"`
	From                  []string `json:"from,omitempty" jsonschema:"发件人邮箱列表"`
	SubjectContains       []string `json:"subjectContains,omitempty" jsonschema:"主题包含关键字列表"`
	MoveTo                string   `json:"moveTo,omitempty" jsonschema:"移动到文件夹名称或 ID"`
	MarkAsRead            bool     `json:"markAsRead,omitempty" jsonschema:"标记为已读"`
	Delete                bool     `json:"delete,omitempty" jsonschema:"删除邮件"`
	Stop                  bool     `json:"stop,omitempty" jsonschema:"停止处理后续规则"`
	Disabled              bool     `json:"disabled,omitempty" jsonschema:"更新为禁用状态"`
	AssignCategories      []string `json:"assignCategories,omitempty" jsonschema:"给邮件添加的类别列表"`
	BodyContains          []string `json:"bodyContains,omitempty" jsonschema:"正文包含关键字列表"`
	BodyOrSubjectContains []string `json:"bodyOrSubjectContains,omitempty" jsonschema:"正文或主题包含关键字列表"`
	HasAttachments        bool     `json:"hasAttachments,omitempty" jsonschema:"匹配带附件的邮件"`
	Importance            string   `json:"importance,omitempty" jsonschema:"匹配邮件重要性: low/normal/high"`
	SenderContains        []string `json:"senderContains,omitempty" jsonschema:"发件人包含关键字列表"`
	RecipientContains     []string `json:"recipientContains,omitempty" jsonschema:"收件人包含关键字列表"`
	CopyTo                string   `json:"copyTo,omitempty" jsonschema:"复制到文件夹名称或 ID"`
	ForwardTo             []string `json:"forwardTo,omitempty" jsonschema:"转发到邮箱列表"`
	RedirectTo            []string `json:"redirectTo,omitempty" jsonschema:"重定向到邮箱列表"`
	MarkImportance        string   `json:"markImportance,omitempty" jsonschema:"设置邮件重要性: low/normal/high"`
	PermanentDelete       bool     `json:"permanentDelete,omitempty" jsonschema:"永久删除邮件"`
}

// mailRuleDeleteArgs 是 mail_rule_delete tool 的输入参数。
type mailRuleDeleteArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	ID      string `json:"id" jsonschema:"规则 ID"`
}

// mailRuleApplyArgs 是 mail_rule_apply tool 的输入参数。
type mailRuleApplyArgs struct {
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	ID      string   `json:"id" jsonschema:"规则 ID"`
	Folder  string   `json:"folder,omitempty" jsonschema:"目标文件夹名称或 ID；不填时使用收件箱"`
	Max     int      `json:"max,omitempty" jsonschema:"最多扫描多少封邮件，默认 500，最大 1000"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"确认实际修改邮件；不填时只 dry-run"`
	Folders []string `json:"folders,omitempty" jsonschema:"目标文件夹名称或 ID 列表"`
}

// mailMasterCategoriesArgs 是 mail_master_categories_list tool 的输入参数。
type mailMasterCategoriesArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailMasterCategoryCreateArgs 是 mail_master_category_create tool 的输入参数。
type mailMasterCategoryCreateArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Name    string `json:"name" jsonschema:"类别名称"`
	Color   string `json:"color,omitempty" jsonschema:"类别颜色，例如 preset0"`
}

// mailMasterCategoryUpdateArgs 是 mail_master_category_update tool 的输入参数。
type mailMasterCategoryUpdateArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	ID      string `json:"id" jsonschema:"类别 ID"`
	Color   string `json:"color" jsonschema:"类别颜色，例如 preset5"`
}

// mailMasterCategoryDeleteArgs 是 mail_master_category_delete tool 的输入参数。
type mailMasterCategoryDeleteArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	ID      string `json:"id" jsonschema:"类别 ID"`
}

// contactsMoveArgs 是 contacts_move tool 的输入参数。
type contactsMoveArgs struct {
	ID      string `json:"id" jsonschema:"联系人 ID"`
	Folder  string `json:"folder" jsonschema:"目标联系人文件夹名称或 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailPermanentDeleteArgs 是 mail_permanent_delete tool 的输入参数。
type mailPermanentDeleteArgs struct {
	ID      string `json:"id" jsonschema:"邮件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsCopyArgs 是 contacts_copy tool 的输入参数。
type contactsCopyArgs struct {
	ID      string `json:"id" jsonschema:"联系人 ID"`
	Folder  string `json:"folder" jsonschema:"目标联系人文件夹名称或 ID"`
	Name    string `json:"name,omitempty" jsonschema:"新联系人显示名称，不填时沿用原名"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarCalendarsArgs 是 calendar_calendars_list tool 的输入参数。
type calendarCalendarsArgs struct {
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsFolderRenameArgs 是 contacts_folder_rename tool 的输入参数。
type contactsFolderRenameArgs struct {
	ID      string `json:"id" jsonschema:"文件夹 ID"`
	Name    string `json:"name" jsonschema:"新的文件夹名称"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDraftUpdateArgs 是 mail_draft_update tool 的输入参数。
type mailDraftUpdateArgs struct {
	ID                     string   `json:"id" jsonschema:"草稿 ID"`
	To                     []string `json:"to" jsonschema:"收件人邮箱列表"`
	Cc                     []string `json:"cc,omitempty" jsonschema:"抄送邮箱列表"`
	Bcc                    []string `json:"bcc,omitempty" jsonschema:"密送邮箱列表"`
	Subject                string   `json:"subject" jsonschema:"邮件主题"`
	Body                   string   `json:"body" jsonschema:"邮件正文"`
	IsHTML                 bool     `json:"isHtml,omitempty" jsonschema:"正文是否为 HTML，默认 false"`
	Attachments            []string `json:"attachments,omitempty" jsonschema:"本地附件文件路径列表"`
	Importance             string   `json:"importance,omitempty" jsonschema:"重要性: low/normal/high"`
	RequestReadReceipt     bool     `json:"requestReadReceipt,omitempty" jsonschema:"请求已读回执"`
	RequestDeliveryReceipt bool     `json:"requestDeliveryReceipt,omitempty" jsonschema:"请求送达回执"`
	Account                string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsPermanentDeleteArgs 是 contacts_permanent_delete tool 的输入参数。
type contactsPermanentDeleteArgs struct {
	ID      string `json:"id" jsonschema:"联系人 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsPhotoGetArgs 是 contacts_photo_get tool 的输入参数。
type contactsPhotoGetArgs struct {
	ID      string `json:"id" jsonschema:"联系人 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsPhotoSetArgs 是 contacts_photo_set tool 的输入参数。
type contactsPhotoSetArgs struct {
	ID      string `json:"id" jsonschema:"联系人 ID"`
	File    string `json:"file" jsonschema:"本地照片文件路径"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarAttachmentsArgs 是 calendar_attachments_list tool 的输入参数。
type calendarAttachmentsArgs struct {
	ID      string `json:"id" jsonschema:"事件 ID"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarAttachmentAddArgs 是 calendar_attachment_add tool 的输入参数。
type calendarAttachmentAddArgs struct {
	ID      string `json:"id" jsonschema:"事件 ID"`
	File    string `json:"file" jsonschema:"本地附件文件路径"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarAttachmentGetArgs 是 calendar_attachment_get tool 的输入参数。
type calendarAttachmentGetArgs struct {
	ID           string `json:"id" jsonschema:"事件 ID"`
	AttachmentID string `json:"attachmentId" jsonschema:"附件 ID"`
	Account      string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarCancelArgs 是 calendar_cancel tool 的输入参数。
type calendarCancelArgs struct {
	ID      string `json:"id" jsonschema:"事件 ID"`
	Comment string `json:"comment,omitempty" jsonschema:"取消说明"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarRespondArgs 是 calendar_respond tool 的输入参数。
type calendarRespondArgs struct {
	ID       string `json:"id" jsonschema:"事件 ID"`
	Response string `json:"response" jsonschema:"响应类型: accept/decline/tentative"`
	Comment  string `json:"comment,omitempty" jsonschema:"响应说明"`
	Account  string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarForwardArgs 是 calendar_forward tool 的输入参数。
type calendarForwardArgs struct {
	ID      string   `json:"id" jsonschema:"事件 ID"`
	To      []string `json:"to" jsonschema:"转发收件人邮箱列表"`
	Comment string   `json:"comment,omitempty" jsonschema:"转发说明"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDraftAttachmentDeleteArgs 是 mail_draft_attachment_delete tool 的输入参数。
type mailDraftAttachmentDeleteArgs struct {
	ID           string `json:"id" jsonschema:"草稿 ID"`
	AttachmentID string `json:"attachmentId" jsonschema:"附件 ID"`
	Account      string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDraftAttachmentReplaceArgs 是 mail_draft_attachment_replace tool 的输入参数。
type mailDraftAttachmentReplaceArgs struct {
	ID           string `json:"id" jsonschema:"草稿 ID"`
	AttachmentID string `json:"attachmentId" jsonschema:"旧附件 ID"`
	File         string `json:"file" jsonschema:"新附件文件路径"`
	Account      string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsExportArgs 是 contacts_export tool 的输入参数。
type contactsExportArgs struct {
	File    string `json:"file" jsonschema:"导出的 CSV 文件路径"`
	Folder  string `json:"folder,omitempty" jsonschema:"联系人文件夹名称或 ID；不填时使用默认文件夹"`
	Max     int    `json:"max,omitempty" jsonschema:"最多导出多少个联系人，默认 500"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Format  string `json:"format,omitempty" jsonschema:"格式: csv 或 vcf"`
}

// contactsImportArgs 是 contacts_import tool 的输入参数。
type contactsImportArgs struct {
	File    string `json:"file" jsonschema:"联系人 CSV 文件路径"`
	Folder  string `json:"folder,omitempty" jsonschema:"目标联系人文件夹名称或 ID；不填时使用默认文件夹"`
	Max     int    `json:"max,omitempty" jsonschema:"最多导入多少个联系人，默认 500"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"确认实际创建联系人；不填时只 dry-run"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
	Format  string `json:"format,omitempty" jsonschema:"格式: csv 或 vcf"`
}

// calendarAttachmentDeleteArgs 是 calendar_attachment_delete tool 的输入参数。
type calendarAttachmentDeleteArgs struct {
	ID           string `json:"id" jsonschema:"事件 ID"`
	AttachmentID string `json:"attachmentId" jsonschema:"附件 ID"`
	Account      string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarAttachmentReplaceArgs 是 calendar_attachment_replace tool 的输入参数。
type calendarAttachmentReplaceArgs struct {
	ID           string `json:"id" jsonschema:"事件 ID"`
	AttachmentID string `json:"attachmentId" jsonschema:"旧附件 ID"`
	File         string `json:"file" jsonschema:"新附件文件路径"`
	Account      string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// auditListArgs 是 audit_list tool 的输入参数。
type auditListArgs struct {
	Limit   int    `json:"limit,omitempty" jsonschema:"最多返回多少条记录，默认 50"`
	Action  string `json:"action,omitempty" jsonschema:"按 action 前缀过滤"`
	Account string `json:"account,omitempty" jsonschema:"按账号过滤"`
	Query   string `json:"query,omitempty" jsonschema:"搜索关键字"`
	Since   string `json:"since,omitempty" jsonschema:"开始时间"`
	Until   string `json:"until,omitempty" jsonschema:"结束时间"`
}

// mailFolderTreeArgs 是 mail_folder_tree tool 的输入参数。
type mailFolderTreeArgs struct {
	Top     int    `json:"top,omitempty" jsonschema:"每层最多读取多少个文件夹，默认 200"`
	Account string `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailBulkMoveArgs 是 mail_bulk_move tool 的输入参数。
type mailBulkMoveArgs struct {
	IDs     []string `json:"ids" jsonschema:"邮件 ID 列表"`
	Folder  string   `json:"folder" jsonschema:"目标文件夹名称或 ID"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"确认实际移动邮件"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailBulkDeleteArgs 是 mail_bulk_delete tool 的输入参数。
type mailBulkDeleteArgs struct {
	IDs     []string `json:"ids" jsonschema:"邮件 ID 列表"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"确认实际删除邮件"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// contactsBulkDeleteArgs 是 contacts_bulk_delete tool 的输入参数。
type contactsBulkDeleteArgs struct {
	IDs     []string `json:"ids" jsonschema:"联系人 ID 列表"`
	Confirm bool     `json:"confirm,omitempty" jsonschema:"确认实际删除联系人"`
	Account string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// calendarAttachmentBulkDeleteArgs 是 calendar_attachment_bulk_delete tool 的输入参数。
type calendarAttachmentBulkDeleteArgs struct {
	ID            string   `json:"id" jsonschema:"事件 ID"`
	AttachmentIDs []string `json:"attachmentIds" jsonschema:"附件 ID 列表"`
	Confirm       bool     `json:"confirm,omitempty" jsonschema:"确认实际删除附件"`
	Account       string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
}

// mailDraftAttachmentBulkDeleteArgs 是 mail_draft_attachment_bulk_delete tool 的输入参数。
type mailDraftAttachmentBulkDeleteArgs struct {
	ID            string   `json:"id" jsonschema:"草稿 ID"`
	AttachmentIDs []string `json:"attachmentIds" jsonschema:"附件 ID 列表"`
	Confirm       bool     `json:"confirm,omitempty" jsonschema:"确认实际删除附件"`
	Account       string   `json:"account,omitempty" jsonschema:"账号邮箱或 HomeAccountID；不填时使用默认账号"`
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

	addTool(server, newTool("account_list", "列出本地已登录的 Outlook 账号档案，不包含 token"), accountListHandler)

	addTool(server, newTool("auth_status", "检查本地 token 缓存，并尝试为每个账号静默刷新 access token"), authStatusHandler)

	addTool(server, newTool("mail_list", "读取指定账号或默认账号收件箱中的最新邮件"), mailListHandler)

	addTool(server, newTool("mail_read", "根据邮件 ID 读取单封邮件详情"), mailReadHandler)

	addTool(server, newTool("mail_search", "使用关键字搜索邮件"), mailSearchHandler)

	addTool(server, newTool("mail_send", "发送一封邮件"), mailSendHandler)

	addTool(server, newTool("mail_folders_list", "列出邮件文件夹"), mailFoldersHandler)

	addTool(server, newTool("mail_folder_create", "创建邮件文件夹"), mailFolderCreateHandler)

	addTool(server, newTool("mail_folder_rename", "重命名邮件文件夹"), mailFolderRenameHandler)

	addTool(server, newTool("mail_folder_delete", "删除邮件文件夹"), mailFolderDeleteHandler)

	addTool(server, newTool("mail_flag", "设置邮件旗标"), mailFlagHandler)

	addTool(server, newTool("mail_unflag", "取消邮件旗标"), mailUnflagHandler)

	addTool(server, newTool("mail_set_importance", "设置邮件重要性"), mailImportanceHandler)

	addTool(server, newTool("mail_message_categories", "列出单封邮件的类别"), mailMessageCategoriesHandler)

	addTool(server, newTool("mail_category_add", "给邮件添加类别"), mailCategoryAddHandler)

	addTool(server, newTool("mail_category_remove", "从邮件移除类别"), mailCategoryRemoveHandler)

	addTool(server, newTool("mail_move", "把邮件移动到指定文件夹"), mailMoveHandler)

	addTool(server, newTool("mail_delete", "删除邮件（移动到已删除邮件）"), mailDeleteHandler)

	addTool(server, newTool("mail_attachments_list", "列出邮件附件"), mailAttachmentsHandler)

	addTool(server, newTool("mail_attachment_save", "下载并保存邮件附件到本地文件"), mailAttachmentSaveHandler)

	addTool(server, newTool("mail_reply", "回复邮件"), mailReplyHandler)

	addTool(server, newTool("mail_reply_all", "回复全部收件人"), mailReplyAllHandler)

	addTool(server, newTool("mail_forward", "转发邮件"), mailForwardHandler)

	addTool(server, newTool("mail_draft_create", "创建草稿邮件"), mailDraftCreateHandler)

	addTool(server, newTool("mail_drafts_list", "列出草稿邮件"), mailDraftsHandler)

	addTool(server, newTool("mail_draft_send", "发送草稿邮件"), mailDraftSendHandler)

	addTool(server, newTool("mail_mark_read", "把指定邮件标记为已读"), mailMarkReadHandler)

	addTool(server, newTool("mail_mark_unread", "把指定邮件标记为未读"), mailMarkUnreadHandler)

	addTool(server, newTool("calendar_list", "读取未来指定天数内的日历事件"), calendarListHandler)

	addTool(server, newTool("contacts_list", "读取联系人列表"), contactsListHandler)

	addTool(server, newTool("calendar_create", "创建日历事件"), calendarCreateHandler)

	addTool(server, newTool("calendar_update", "更新日历事件"), calendarUpdateHandler)

	addTool(server, newTool("calendar_delete", "删除日历事件"), calendarDeleteHandler)

	addTool(server, newTool("contacts_create", "创建联系人"), contactsCreateHandler)

	addTool(server, newTool("contacts_update", "更新联系人"), contactsUpdateHandler)

	addTool(server, newTool("contacts_delete", "删除联系人"), contactsDeleteHandler)
	addTool(server, newTool("contacts_search", "按显示名称前缀搜索联系人"), contactsSearchHandler)

	addTool(server, newTool("contacts_folders_list", "列出联系人文件夹"), contactsFoldersHandler)

	addTool(server, newTool("contacts_folder_create", "创建联系人文件夹"), contactsFolderCreateHandler)

	addTool(server, newTool("contacts_folder_delete", "删除联系人文件夹"), contactsFolderDeleteHandler)
	addTool(server, newTool("mail_rules_list", "列出收件箱规则"), mailRulesHandler)

	addTool(server, newTool("mail_rule_create", "创建收件箱规则"), mailRuleCreateHandler)

	addTool(server, newTool("mail_rule_delete", "删除收件箱规则"), mailRuleDeleteHandler)

	addTool(server, newTool("mail_master_categories_list", "列出主类别"), mailMasterCategoriesHandler)

	addTool(server, newTool("mail_master_category_create", "创建主类别"), mailMasterCategoryCreateHandler)

	addTool(server, newTool("mail_master_category_delete", "删除主类别"), mailMasterCategoryDeleteHandler)
	addTool(server, newTool("contacts_move", "把联系人移动到指定文件夹"), contactsMoveHandler)

	addTool(server, newTool("mail_permanent_delete", "永久删除邮件"), mailPermanentDeleteHandler)

	addTool(server, newTool("contacts_copy", "复制联系人到指定文件夹"), contactsCopyHandler)

	addTool(server, newTool("calendar_calendars_list", "列出日历"), calendarCalendarsHandler)

	addTool(server, newTool("contacts_folder_rename", "重命名联系人文件夹"), contactsFolderRenameHandler)

	addTool(server, newTool("mail_draft_update", "更新草稿邮件"), mailDraftUpdateHandler)

	addTool(server, newTool("contacts_permanent_delete", "永久删除联系人"), contactsPermanentDeleteHandler)

	addTool(server, newTool("contacts_photo_get", "读取联系人照片"), contactsPhotoGetHandler)

	addTool(server, newTool("contacts_photo_set", "设置联系人照片"), contactsPhotoSetHandler)

	addTool(server, newTool("calendar_attachments_list", "列出日历事件附件"), calendarAttachmentsHandler)

	addTool(server, newTool("calendar_attachment_add", "给日历事件添加附件"), calendarAttachmentAddHandler)

	addTool(server, newTool("calendar_attachment_get", "下载日历事件附件"), calendarAttachmentGetHandler)

	addTool(server, newTool("mail_rule_update", "更新收件箱规则"), mailRuleUpdateHandler)

	addTool(server, newTool("mail_master_category_update", "更新主类别颜色"), mailMasterCategoryUpdateHandler)

	addTool(server, newTool("calendar_cancel", "取消日历事件"), calendarCancelHandler)

	addTool(server, newTool("calendar_respond", "响应日历邀请"), calendarRespondHandler)

	addTool(server, newTool("mail_rule_apply", "把收件箱规则回放到已有邮件，默认 dry-run"), mailRuleApplyHandler)

	addTool(server, newTool("calendar_forward", "转发日历事件"), calendarForwardHandler)

	addTool(server, newTool("mail_draft_attachment_delete", "删除草稿附件"), mailDraftAttachmentDeleteHandler)

	addTool(server, newTool("mail_draft_attachment_replace", "替换草稿附件"), mailDraftAttachmentReplaceHandler)

	addTool(server, newTool("contacts_export", "导出联系人到 CSV"), contactsExportHandler)

	addTool(server, newTool("contacts_import", "从 CSV 导入联系人"), contactsImportHandler)

	addTool(server, newTool("calendar_attachment_delete", "删除日历事件附件"), calendarAttachmentDeleteHandler)

	addTool(server, newTool("calendar_attachment_replace", "替换日历事件附件"), calendarAttachmentReplaceHandler)

	addTool(server, newTool("audit_list", "查询本地操作审计日志"), auditListHandler)

	addTool(server, newTool("mail_folder_tree", "以树形列出邮件文件夹"), mailFolderTreeHandler)

	addTool(server, newTool("mail_bulk_move", "批量移动邮件"), mailBulkMoveHandler)

	addTool(server, newTool("mail_bulk_delete", "批量删除邮件"), mailBulkDeleteHandler)

	addTool(server, newTool("contacts_bulk_delete", "批量删除联系人"), contactsBulkDeleteHandler)

	addTool(server, newTool("calendar_attachment_bulk_delete", "批量删除日历事件附件"), calendarAttachmentBulkDeleteHandler)

	addTool(server, newTool("mail_draft_attachment_bulk_delete", "批量删除草稿附件"), mailDraftAttachmentBulkDeleteHandler)

	registerMailboxTools(server)
	registerBulkStatusTools(server)
	registerCalendarExtraTools(server)
	registerContactExtraTools(server)
	registerIndexTools(server)
	registerEMLTools(server)
	registerAuditTools(server)
	registerMailQueryTools(server)

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
	if args.All {
		folderID := "inbox"
		if strings.TrimSpace(args.Folder) != "" {
			resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, args.Folder)
			if resolveErr != nil {
				return nil, nil, resolveErr
			}
			folderID = resolved
		}
		messages, err = graph.ListMessagesInFolderAll(ctx, loginResult.AccessToken, folderID, args.Max)
	} else if strings.TrimSpace(args.Folder) == "" {
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
	attachments, err := graph.LoadMailAttachments(args.Attachments)
	if err != nil {
		return nil, nil, err
	}

	input := graph.SendMailInput{
		To:                     args.To,
		Cc:                     args.Cc,
		Bcc:                    args.Bcc,
		Subject:                args.Subject,
		Body:                   args.Body,
		ContentType:            contentType,
		Attachments:            attachments,
		Importance:             args.Importance,
		RequestReadReceipt:     args.RequestReadReceipt,
		RequestDeliveryReceipt: args.RequestDeliveryReceipt,
		SaveToSentItems:        args.SaveToSentItems,
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
	attachments, err := graph.LoadMailAttachments(args.Attachments)
	if err != nil {
		return nil, nil, err
	}

	draftID, err := graph.CreateDraft(ctx, loginResult.AccessToken, graph.SendMailInput{
		To:                     args.To,
		Cc:                     args.Cc,
		Bcc:                    args.Bcc,
		Subject:                args.Subject,
		Body:                   args.Body,
		ContentType:            contentType,
		Attachments:            attachments,
		Importance:             args.Importance,
		RequestReadReceipt:     args.RequestReadReceipt,
		RequestDeliveryReceipt: args.RequestDeliveryReceipt,
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
	calendarID, err := graph.ResolveCalendarID(ctx, loginResult.AccessToken, args.Calendar)
	if err != nil {
		return nil, nil, err
	}
	events, err := graph.ListCalendarEventsInCalendar(ctx, loginResult.AccessToken, calendarID, start, end, args.Top)
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
		Categories:                 args.Categories,
		ShowAs:                     args.ShowAs,
		Importance:                 args.Importance,
		Sensitivity:                args.Sensitivity,
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
		Categories:                 args.Categories,
		ShowAs:                     args.ShowAs,
		Importance:                 args.Importance,
		Sensitivity:                args.Sensitivity,
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
	var birthday string
	if strings.TrimSpace(args.Birthday) != "" {
		parsed, parseErr := parseCalendarTimeValue(args.Birthday)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		birthday = parsed.Format(time.RFC3339)
	}

	input := graph.ContactInput{
		DisplayName:      args.DisplayName,
		GivenName:        args.GivenName,
		Surname:          args.Surname,
		CompanyName:      args.CompanyName,
		JobTitle:         args.JobTitle,
		MobilePhone:      args.MobilePhone,
		NickName:         args.NickName,
		Birthday:         birthday,
		PersonalNotes:    args.PersonalNotes,
		Department:       args.Department,
		OfficeLocation:   args.OfficeLocation,
		Profession:       args.Profession,
		BusinessHomePage: args.BusinessHomePage,
		HomeAddress:      args.HomeAddress,
		BusinessAddress:  args.BusinessAddress,
		OtherAddress:     args.OtherAddress,
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
	var birthday string
	if strings.TrimSpace(args.Birthday) != "" {
		parsed, parseErr := parseCalendarTimeValue(args.Birthday)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		birthday = parsed.Format(time.RFC3339)
	}

	input := graph.ContactInput{
		DisplayName:      args.DisplayName,
		GivenName:        args.GivenName,
		Surname:          args.Surname,
		CompanyName:      args.CompanyName,
		JobTitle:         args.JobTitle,
		MobilePhone:      args.MobilePhone,
		NickName:         args.NickName,
		Birthday:         birthday,
		PersonalNotes:    args.PersonalNotes,
		Department:       args.Department,
		OfficeLocation:   args.OfficeLocation,
		Profession:       args.Profession,
		BusinessHomePage: args.BusinessHomePage,
		HomeAddress:      args.HomeAddress,
		BusinessAddress:  args.BusinessAddress,
		OtherAddress:     args.OtherAddress,
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

// mailRulesHandler 列出收件箱规则。
func mailRulesHandler(ctx context.Context, req *mcp.CallToolRequest, args mailRulesArgs) (*mcp.CallToolResult, any, error) {
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
	rules, err := graph.ListMessageRules(ctx, loginResult.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account": selected.Username,
		"rules":   rules,
	})
}

// mailRuleCreateHandler 创建收件箱规则。
func mailRuleCreateHandler(ctx context.Context, req *mcp.CallToolRequest, args mailRuleCreateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Name) == "" {
		return nil, nil, fmt.Errorf("规则名称不能为空")
	}
	if len(args.From) == 0 && len(args.SubjectContains) == 0 && len(args.BodyContains) == 0 && len(args.BodyOrSubjectContains) == 0 && !args.HasAttachments && strings.TrimSpace(args.Importance) == "" && len(args.SenderContains) == 0 && len(args.RecipientContains) == 0 {
		return nil, nil, fmt.Errorf("至少需要一个规则条件")
	}
	if strings.TrimSpace(args.MoveTo) == "" && strings.TrimSpace(args.CopyTo) == "" && len(args.AssignCategories) == 0 && len(args.ForwardTo) == 0 && len(args.RedirectTo) == 0 && !args.MarkAsRead && strings.TrimSpace(args.MarkImportance) == "" && !args.Delete && !args.PermanentDelete && !args.Stop {
		return nil, nil, fmt.Errorf("至少需要一个规则动作")
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
	moveTo := strings.TrimSpace(args.MoveTo)
	if moveTo != "" {
		moveTo, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, moveTo)
		if err != nil {
			return nil, nil, err
		}
	}
	copyTo := strings.TrimSpace(args.CopyTo)
	if copyTo != "" {
		copyTo, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, copyTo)
		if err != nil {
			return nil, nil, err
		}
	}
	rule, err := graph.CreateMessageRule(ctx, loginResult.AccessToken, graph.MessageRuleInput{
		DisplayName:           args.Name,
		FromAddresses:         args.From,
		SubjectContains:       args.SubjectContains,
		MoveToFolder:          moveTo,
		AssignCategories:      args.AssignCategories,
		BodyContains:          args.BodyContains,
		BodyOrSubjectContains: args.BodyOrSubjectContains,
		HasAttachments:        args.HasAttachments,
		Importance:            args.Importance,
		SenderContains:        args.SenderContains,
		RecipientContains:     args.RecipientContains,
		CopyToFolder:          copyTo,
		ForwardTo:             args.ForwardTo,
		RedirectTo:            args.RedirectTo,
		MarkImportance:        args.MarkImportance,
		PermanentDelete:       args.PermanentDelete,
		MarkAsRead:            args.MarkAsRead,
		Delete:                args.Delete,
		StopProcessingRules:   args.Stop,
		IsEnabled:             !args.Disabled,
	})
	if err != nil {
		return nil, nil, err
	}
	var applyResult *graph.RuleApplyResult
	if args.ApplyNow {
		folderIDs := make([]string, 0)
		if len(args.ApplyFolders) > 0 {
			for _, folder := range args.ApplyFolders {
				folder = strings.TrimSpace(folder)
				if folder == "" {
					continue
				}
				resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, folder)
				if resolveErr != nil {
					return nil, nil, resolveErr
				}
				folderIDs = append(folderIDs, resolved)
			}
		} else {
			folderID := "inbox"
			if strings.TrimSpace(args.ApplyFolder) != "" {
				resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, args.ApplyFolder)
				if resolveErr != nil {
					return nil, nil, resolveErr
				}
				folderID = resolved
			}
			folderIDs = append(folderIDs, folderID)
		}
		applyResult, err = graph.ApplyMessageRuleToFolders(ctx, loginResult.AccessToken, rule, folderIDs, args.ApplyMax, args.ConfirmApply)
		if err != nil {
			return nil, nil, err
		}
	}
	result := map[string]any{
		"success": true,
		"account": selected.Username,
		"rule":    rule,
	}
	if applyResult != nil {
		result["applyResult"] = applyResult
	}
	return jsonResult(result)
}

// mailRuleUpdateHandler 更新收件箱规则。
func mailRuleUpdateHandler(ctx context.Context, req *mcp.CallToolRequest, args mailRuleUpdateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.Name) == "" {
		return nil, nil, fmt.Errorf("规则 ID 和名称不能为空")
	}
	if len(args.From) == 0 && len(args.SubjectContains) == 0 && len(args.BodyContains) == 0 && len(args.BodyOrSubjectContains) == 0 && !args.HasAttachments && strings.TrimSpace(args.Importance) == "" && len(args.SenderContains) == 0 && len(args.RecipientContains) == 0 {
		return nil, nil, fmt.Errorf("至少需要一个规则条件")
	}
	if strings.TrimSpace(args.MoveTo) == "" && strings.TrimSpace(args.CopyTo) == "" && len(args.AssignCategories) == 0 && len(args.ForwardTo) == 0 && len(args.RedirectTo) == 0 && !args.MarkAsRead && strings.TrimSpace(args.MarkImportance) == "" && !args.Delete && !args.PermanentDelete && !args.Stop {
		return nil, nil, fmt.Errorf("至少需要一个规则动作")
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
	moveTo := strings.TrimSpace(args.MoveTo)
	if moveTo != "" {
		moveTo, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, moveTo)
		if err != nil {
			return nil, nil, err
		}
	}
	copyTo := strings.TrimSpace(args.CopyTo)
	if copyTo != "" {
		copyTo, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, copyTo)
		if err != nil {
			return nil, nil, err
		}
	}
	rule, err := graph.UpdateMessageRule(ctx, loginResult.AccessToken, args.ID, graph.MessageRuleInput{
		DisplayName:           args.Name,
		FromAddresses:         args.From,
		SubjectContains:       args.SubjectContains,
		MoveToFolder:          moveTo,
		AssignCategories:      args.AssignCategories,
		BodyContains:          args.BodyContains,
		BodyOrSubjectContains: args.BodyOrSubjectContains,
		HasAttachments:        args.HasAttachments,
		Importance:            args.Importance,
		SenderContains:        args.SenderContains,
		RecipientContains:     args.RecipientContains,
		CopyToFolder:          copyTo,
		ForwardTo:             args.ForwardTo,
		RedirectTo:            args.RedirectTo,
		MarkImportance:        args.MarkImportance,
		PermanentDelete:       args.PermanentDelete,
		MarkAsRead:            args.MarkAsRead,
		Delete:                args.Delete,
		StopProcessingRules:   args.Stop,
		IsEnabled:             !args.Disabled,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"rule":    rule,
	})
}

// mailRuleApplyHandler 把一条收件箱规则回放到已有邮件。
// mailRuleApplyHandler 把一条收件箱规则回放到已有邮件。
func mailRuleApplyHandler(ctx context.Context, req *mcp.CallToolRequest, args mailRuleApplyArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("规则 ID 不能为空")
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
	rule, err := graph.GetMessageRule(ctx, loginResult.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	folderIDs := make([]string, 0)
	if len(args.Folders) > 0 {
		for _, folder := range args.Folders {
			folder = strings.TrimSpace(folder)
			if folder == "" {
				continue
			}
			resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, folder)
			if resolveErr != nil {
				return nil, nil, resolveErr
			}
			folderIDs = append(folderIDs, resolved)
		}
	} else {
		folderID := "inbox"
		if strings.TrimSpace(args.Folder) != "" {
			folderID, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, args.Folder)
			if err != nil {
				return nil, nil, err
			}
		}
		folderIDs = append(folderIDs, folderID)
	}
	result, err := graph.ApplyMessageRuleToFolders(ctx, loginResult.AccessToken, rule, folderIDs, args.Max, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":     true,
		"account":     selected.Username,
		"applyResult": result,
	})
}

// mailRuleDeleteHandler 删除收件箱规则。
func mailRuleDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailRuleDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("规则 ID 不能为空")
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
	if err := graph.DeleteMessageRule(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"ruleId":  args.ID,
	})
}

// mailMasterCategoriesHandler 列出主类别。
func mailMasterCategoriesHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMasterCategoriesArgs) (*mcp.CallToolResult, any, error) {
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
	categories, err := graph.ListMasterCategories(ctx, loginResult.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account":    selected.Username,
		"categories": categories,
	})
}

// mailMasterCategoryCreateHandler 创建主类别。
func mailMasterCategoryCreateHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMasterCategoryCreateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.Name) == "" {
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
	category, err := graph.CreateMasterCategory(ctx, loginResult.AccessToken, args.Name, args.Color)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"category": category,
	})
}

// mailMasterCategoryUpdateHandler 更新主类别。
func mailMasterCategoryUpdateHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMasterCategoryUpdateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.Color) == "" {
		return nil, nil, fmt.Errorf("类别 ID 和颜色不能为空")
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
	category, err := graph.UpdateMasterCategory(ctx, loginResult.AccessToken, args.ID, args.Color)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"category": category,
	})
}

// mailMasterCategoryDeleteHandler 删除主类别。
func mailMasterCategoryDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailMasterCategoryDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("类别 ID 不能为空")
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
	if err := graph.DeleteMasterCategory(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":    true,
		"account":    selected.Username,
		"categoryId": args.ID,
	})
}

// contactsMoveHandler 把联系人移动到指定文件夹。
func contactsMoveHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsMoveArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.Folder) == "" {
		return nil, nil, fmt.Errorf("联系人 ID 和目标文件夹不能为空")
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
	folderID, err := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, args.Folder)
	if err != nil {
		return nil, nil, err
	}
	moved, err := graph.MoveContact(ctx, loginResult.AccessToken, args.ID, folderID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":      true,
		"account":      selected.Username,
		"oldContactId": args.ID,
		"newContact":   moved,
		"folder":       args.Folder,
	})
}

// mailPermanentDeleteHandler 永久删除邮件。
func mailPermanentDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailPermanentDeleteArgs) (*mcp.CallToolResult, any, error) {
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
	if err := graph.PermanentDeleteMessage(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"messageId": args.ID,
	})
}

// contactsCopyHandler 复制联系人到指定文件夹。
func contactsCopyHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsCopyArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.Folder) == "" {
		return nil, nil, fmt.Errorf("联系人 ID 和目标文件夹不能为空")
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
	folderID, err := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, args.Folder)
	if err != nil {
		return nil, nil, err
	}
	copied, err := graph.CopyContact(ctx, loginResult.AccessToken, args.ID, folderID, args.Name)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":       true,
		"account":       selected.Username,
		"sourceId":      args.ID,
		"copiedContact": copied,
		"folder":        args.Folder,
	})
}

// calendarCalendarsHandler 列出日历。
func calendarCalendarsHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarCalendarsArgs) (*mcp.CallToolResult, any, error) {
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
	calendars, err := graph.ListCalendars(ctx, loginResult.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"account":   selected.Username,
		"calendars": calendars,
	})
}

// contactsFolderRenameHandler 重命名联系人文件夹。
func contactsFolderRenameHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsFolderRenameArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.Name) == "" {
		return nil, nil, fmt.Errorf("文件夹 ID 和名称不能为空")
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
	if err := graph.RenameContactFolder(ctx, loginResult.AccessToken, args.ID, args.Name); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"folderId": args.ID,
		"newName":  args.Name,
	})
}

// mailDraftUpdateHandler 更新草稿邮件。
func mailDraftUpdateHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDraftUpdateArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" {
		return nil, nil, fmt.Errorf("草稿 ID 不能为空")
	}
	if len(args.To) == 0 {
		return nil, nil, fmt.Errorf("至少需要一个收件人")
	}
	attachments, err := graph.LoadMailAttachments(args.Attachments)
	if err != nil {
		return nil, nil, err
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
	detail, err := graph.UpdateDraft(ctx, loginResult.AccessToken, args.ID, graph.SendMailInput{
		To:                     args.To,
		Cc:                     args.Cc,
		Bcc:                    args.Bcc,
		Subject:                args.Subject,
		Body:                   args.Body,
		ContentType:            contentType,
		Attachments:            attachments,
		Importance:             args.Importance,
		RequestReadReceipt:     args.RequestReadReceipt,
		RequestDeliveryReceipt: args.RequestDeliveryReceipt,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"draft":   detail,
	})
}

// contactsPermanentDeleteHandler 永久删除联系人。
func contactsPermanentDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsPermanentDeleteArgs) (*mcp.CallToolResult, any, error) {
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
	if err := graph.PermanentDeleteContact(ctx, loginResult.AccessToken, args.ID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":   true,
		"account":   selected.Username,
		"contactId": args.ID,
	})
}

// contactsPhotoGetHandler 读取联系人照片。
func contactsPhotoGetHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsPhotoGetArgs) (*mcp.CallToolResult, any, error) {
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
	data, contentType, err := graph.GetContactPhoto(ctx, loginResult.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":     true,
		"account":     selected.Username,
		"contactId":   args.ID,
		"contentType": contentType,
		"size":        len(data),
		"base64":      base64.StdEncoding.EncodeToString(data),
	})
}

// contactsPhotoSetHandler 设置联系人照片。
func contactsPhotoSetHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsPhotoSetArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("联系人 ID 和照片文件不能为空")
	}
	data, err := os.ReadFile(args.File)
	if err != nil {
		return nil, nil, fmt.Errorf("读取联系人照片失败: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(args.File))
	if contentType == "" {
		contentType = "image/jpeg"
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
	if err := graph.SetContactPhoto(ctx, loginResult.AccessToken, args.ID, data, contentType); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":     true,
		"account":     selected.Username,
		"contactId":   args.ID,
		"contentType": contentType,
		"size":        len(data),
	})
}

// calendarAttachmentsHandler 列出日历事件附件。
func calendarAttachmentsHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarAttachmentsArgs) (*mcp.CallToolResult, any, error) {
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
	attachments, err := graph.ListCalendarEventAttachments(ctx, loginResult.AccessToken, args.ID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":     true,
		"account":     selected.Username,
		"eventId":     args.ID,
		"attachments": attachments,
	})
}

// calendarAttachmentAddHandler 给日历事件添加附件。
func calendarAttachmentAddHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarAttachmentAddArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("事件 ID 和附件文件不能为空")
	}
	attachments, err := graph.LoadMailAttachments([]string{args.File})
	if err != nil {
		return nil, nil, err
	}
	if len(attachments) == 0 {
		return nil, nil, fmt.Errorf("附件文件不能为空")
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
	attachment, err := graph.AddCalendarEventAttachment(ctx, loginResult.AccessToken, args.ID, attachments[0])
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":    true,
		"account":    selected.Username,
		"eventId":    args.ID,
		"attachment": attachment,
	})
}

// calendarAttachmentGetHandler 下载日历事件附件。
func calendarAttachmentGetHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarAttachmentGetArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.AttachmentID) == "" {
		return nil, nil, fmt.Errorf("事件 ID 和附件 ID 不能为空")
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
	data, contentType, err := graph.GetCalendarEventAttachment(ctx, loginResult.AccessToken, args.ID, args.AttachmentID)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":      true,
		"account":      selected.Username,
		"eventId":      args.ID,
		"attachmentId": args.AttachmentID,
		"contentType":  contentType,
		"size":         len(data),
		"base64":       base64.StdEncoding.EncodeToString(data),
	})
}

// calendarCancelHandler 取消日历事件。
func calendarCancelHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarCancelArgs) (*mcp.CallToolResult, any, error) {
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
	if err := graph.CancelCalendarEvent(ctx, loginResult.AccessToken, args.ID, args.Comment); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"eventId": args.ID,
	})
}

// calendarRespondHandler 响应日历邀请。
func calendarRespondHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarRespondArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.Response) == "" {
		return nil, nil, fmt.Errorf("事件 ID 和响应类型不能为空")
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
	if err := graph.RespondCalendarEvent(ctx, loginResult.AccessToken, args.ID, args.Response, args.Comment); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"eventId":  args.ID,
		"response": args.Response,
	})
}

// calendarForwardHandler 转发日历事件。
func calendarForwardHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarForwardArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || len(args.To) == 0 {
		return nil, nil, fmt.Errorf("事件 ID 和转发收件人不能为空")
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
	if err := graph.ForwardCalendarEvent(ctx, loginResult.AccessToken, args.ID, args.Comment, args.To); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"eventId": args.ID,
		"to":      args.To,
	})
}

// mailDraftAttachmentDeleteHandler 删除草稿附件。
func mailDraftAttachmentDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDraftAttachmentDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.AttachmentID) == "" {
		return nil, nil, fmt.Errorf("草稿 ID 和附件 ID 不能为空")
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
	if err := graph.DeleteDraftAttachment(ctx, loginResult.AccessToken, args.ID, args.AttachmentID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":      true,
		"account":      selected.Username,
		"draftId":      args.ID,
		"attachmentId": args.AttachmentID,
	})
}

// mailDraftAttachmentReplaceHandler 替换草稿附件。
func mailDraftAttachmentReplaceHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDraftAttachmentReplaceArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.AttachmentID) == "" || strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("草稿 ID、旧附件 ID 和新附件文件不能为空")
	}
	attachments, err := graph.LoadMailAttachments([]string{args.File})
	if err != nil {
		return nil, nil, err
	}
	if len(attachments) == 0 {
		return nil, nil, fmt.Errorf("新附件文件不能为空")
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
	attachment, err := graph.ReplaceDraftAttachment(ctx, loginResult.AccessToken, args.ID, args.AttachmentID, attachments[0])
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":    true,
		"account":    selected.Username,
		"draftId":    args.ID,
		"attachment": attachment,
	})
}

// contactsExportHandler 导出联系人到 CSV。
func contactsExportHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsExportArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("导出文件路径不能为空")
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
	folderID := ""
	if strings.TrimSpace(args.Folder) != "" {
		folderID, err = graph.ResolveContactFolderID(ctx, loginResult.AccessToken, args.Folder)
		if err != nil {
			return nil, nil, err
		}
	}
	contacts, err := graph.ListContactsInFolderAll(ctx, loginResult.AccessToken, folderID, args.Max)
	if err != nil {
		return nil, nil, err
	}
	switch strings.ToLower(strings.TrimSpace(args.Format)) {
	case "csv", "":
		if err := contactcsv.ExportFile(args.File, contacts); err != nil {
			return nil, nil, err
		}
	case "vcf", "vcard":
		if err := vcard.ExportFile(args.File, contacts); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("不支持的导出格式: %s", args.Format)
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"count":   len(contacts),
		"file":    args.File,
	})
}

// contactsImportHandler 从 CSV 导入联系人。
func contactsImportHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsImportArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("导入文件路径不能为空")
	}
	var inputs []graph.ContactInput
	var err error
	switch strings.ToLower(strings.TrimSpace(args.Format)) {
	case "csv", "":
		inputs, err = contactcsv.ReadFile(args.File)
	case "vcf", "vcard":
		inputs, err = vcard.ReadFile(args.File)
	default:
		return nil, nil, fmt.Errorf("不支持的导入格式: %s", args.Format)
	}
	if err != nil {
		return nil, nil, err
	}
	max := args.Max
	if max <= 0 {
		max = 500
	}
	if max > 1000 {
		max = 1000
	}
	if len(inputs) > max {
		inputs = inputs[:max]
	}
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
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
	folderID := ""
	if strings.TrimSpace(args.Folder) != "" {
		folderID, err = graph.ResolveContactFolderID(ctx, loginResult.AccessToken, args.Folder)
		if err != nil {
			return nil, nil, err
		}
	}
	if !args.Confirm {
		return jsonResult(map[string]any{
			"success": true,
			"account": selected.Username,
			"dryRun":  true,
			"count":   len(inputs),
			"file":    args.File,
		})
	}
	successCount := 0
	failureCount := 0
	failures := make([]string, 0)
	for _, input := range inputs {
		if _, err := graph.CreateContactInFolder(ctx, loginResult.AccessToken, folderID, input); err != nil {
			failureCount++
			failures = append(failures, fmt.Sprintf("%s: %v", input.DisplayName, err))
			continue
		}
		successCount++
	}
	return jsonResult(map[string]any{
		"success":  true,
		"account":  selected.Username,
		"dryRun":   false,
		"imported": successCount,
		"failed":   failureCount,
		"failures": failures,
		"file":     args.File,
	})
}

// calendarAttachmentDeleteHandler 删除日历事件附件。
func calendarAttachmentDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarAttachmentDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.AttachmentID) == "" {
		return nil, nil, fmt.Errorf("事件 ID 和附件 ID 不能为空")
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
	if err := graph.DeleteCalendarEventAttachment(ctx, loginResult.AccessToken, args.ID, args.AttachmentID); err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":      true,
		"account":      selected.Username,
		"eventId":      args.ID,
		"attachmentId": args.AttachmentID,
	})
}

// calendarAttachmentReplaceHandler 替换日历事件附件。
func calendarAttachmentReplaceHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarAttachmentReplaceArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || strings.TrimSpace(args.AttachmentID) == "" || strings.TrimSpace(args.File) == "" {
		return nil, nil, fmt.Errorf("事件 ID、旧附件 ID 和新附件文件不能为空")
	}
	attachments, err := graph.LoadMailAttachments([]string{args.File})
	if err != nil {
		return nil, nil, err
	}
	if len(attachments) == 0 {
		return nil, nil, fmt.Errorf("新附件文件不能为空")
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
	attachment, err := graph.ReplaceCalendarEventAttachment(ctx, loginResult.AccessToken, args.ID, args.AttachmentID, attachments[0])
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success":    true,
		"account":    selected.Username,
		"eventId":    args.ID,
		"attachment": attachment,
	})
}

// auditListHandler 查询本地审计日志。
func auditListHandler(ctx context.Context, req *mcp.CallToolRequest, args auditListArgs) (*mcp.CallToolResult, any, error) {
	query, err := auditQueryFromArgs(auditQueryArgs{Limit: args.Limit, Action: args.Action, Account: args.Account, Query: args.Query, Since: args.Since, Until: args.Until})
	if err != nil {
		return nil, nil, err
	}
	entries, err := audit.Query(query)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}

// mailFolderTreeHandler 以树形列出邮件文件夹。
func mailFolderTreeHandler(ctx context.Context, req *mcp.CallToolRequest, args mailFolderTreeArgs) (*mcp.CallToolResult, any, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
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
	tree, err := graph.ListMailFolderTree(ctx, loginResult.AccessToken, args.Top)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{
		"success": true,
		"account": selected.Username,
		"folders": tree,
	})
}

func mailBulkMoveHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkMoveArgs) (*mcp.CallToolResult, any, error) {
	if len(args.IDs) == 0 || strings.TrimSpace(args.Folder) == "" {
		return nil, nil, fmt.Errorf("邮件 ID 列表和目标文件夹不能为空")
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
	folderID, err := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, args.Folder)
	if err != nil {
		return nil, nil, err
	}
	result, err := graph.BulkMoveMessages(ctx, loginResult.AccessToken, args.IDs, folderID, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": selected.Username, "result": result})
}

func mailBulkDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailBulkDeleteArgs) (*mcp.CallToolResult, any, error) {
	if len(args.IDs) == 0 {
		return nil, nil, fmt.Errorf("邮件 ID 列表不能为空")
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
	result, err := graph.BulkDeleteMessages(ctx, loginResult.AccessToken, args.IDs, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": selected.Username, "result": result})
}

func contactsBulkDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args contactsBulkDeleteArgs) (*mcp.CallToolResult, any, error) {
	if len(args.IDs) == 0 {
		return nil, nil, fmt.Errorf("联系人 ID 列表不能为空")
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
	result, err := graph.BulkDeleteContacts(ctx, loginResult.AccessToken, args.IDs, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": selected.Username, "result": result})
}

func calendarAttachmentBulkDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args calendarAttachmentBulkDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || len(args.AttachmentIDs) == 0 {
		return nil, nil, fmt.Errorf("事件 ID 和附件 ID 列表不能为空")
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
	result, err := graph.BulkDeleteCalendarEventAttachments(ctx, loginResult.AccessToken, args.ID, args.AttachmentIDs, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": selected.Username, "result": result})
}

func mailDraftAttachmentBulkDeleteHandler(ctx context.Context, req *mcp.CallToolRequest, args mailDraftAttachmentBulkDeleteArgs) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(args.ID) == "" || len(args.AttachmentIDs) == 0 {
		return nil, nil, fmt.Errorf("草稿 ID 和附件 ID 列表不能为空")
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
	result, err := graph.BulkDeleteDraftAttachments(ctx, loginResult.AccessToken, args.ID, args.AttachmentIDs, args.Confirm)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"success": true, "account": selected.Username, "result": result})
}

// toolErrorResult 把 handler 错误转换为统一的 MCP structured error。
func toolErrorResult(err error) *mcp.CallToolResult {
	code := classifyToolError(err)
	payload := map[string]any{
		"success": false,
		"error": map[string]any{
			"code":    code,
			"message": err.Error(),
		},
	}
	data, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		data = []byte(`{"success":false,"error":{"code":"internal_error","message":"failed to marshal error"}}`)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
		StructuredContent: payload,
		IsError:           true,
	}
}

func classifyToolError(err error) string {
	if err == nil {
		return "unknown"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "401"), strings.Contains(message, "unauthorized"):
		return "unauthorized"
	case strings.Contains(message, "403"), strings.Contains(message, "forbidden"):
		return "forbidden"
	case strings.Contains(message, "404"), strings.Contains(message, "not found"), strings.Contains(message, "找不到"):
		return "not_found"
	case strings.Contains(message, "429"), strings.Contains(message, "too many requests"), strings.Contains(message, "throttled"):
		return "rate_limited"
	case strings.Contains(message, "400"), strings.Contains(message, "bad request"), strings.Contains(message, "不能为空"), strings.Contains(message, "至少需要"):
		return "invalid_argument"
	case strings.Contains(message, "timeout"), strings.Contains(message, "context deadline"):
		return "timeout"
	case strings.Contains(message, "network"), strings.Contains(message, "connection"), strings.Contains(message, "tls"):
		return "network_error"
	case strings.Contains(message, "刷新 access token"), strings.Contains(message, "token"):
		return "auth_error"
	default:
		return "graph_error"
	}
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
		StructuredContent: map[string]any{"result": value},
	}, nil, nil
}
