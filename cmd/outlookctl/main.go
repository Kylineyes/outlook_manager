// Package main 是 outlookctl 命令行工具的入口。
//
// 当前阶段实现认证、账号管理和邮件读取的最小闭环：
//   - auth login：通过 Device Code 登录并加密保存 token。
//   - auth status：验证本地 token 缓存是否可以静默刷新。
//   - account list：查看可读账号档案，便于人工确认。
//   - account use：切换默认账号。
//   - mail list：读取当前账号收件箱最新邮件。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/auth"
	"github.com/Kylineyes/outlook-manager/internal/contactcsv"
	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
	"github.com/Kylineyes/outlook-manager/internal/vcard"
)

// version 表示当前构建版本。
// 默认值用于本地开发；正式发布时由 GitHub Actions 通过 -ldflags 注入 tag。
var version = "dev"

// main 负责捕获错误并设置进程退出码。
func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "outlookctl: %v\n", err)
		os.Exit(1)
	}
}

// parseCommandFlags 解析子命令参数，并在 --help / -h 时输出该子命令的参数列表。
func parseCommandFlags(fs *flag.FlagSet, args []string) error {
	fs.Usage = func() {
		fmt.Fprintf(os.Stdout, "用法: outlookctl %s [options]\n\n选项:\n", fs.Name())
		fs.SetOutput(os.Stdout)
		fs.PrintDefaults()
	}
	return fs.Parse(args)
}

// helpRequested 判断参数中是否包含 --help 或 -h。
func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// printHelpAndReturn 输出子命令用法并返回 flag.ErrHelp，让 main 以成功状态退出。
func printHelpAndReturn(usage string) error {
	fmt.Fprintln(os.Stdout, usage)
	return flag.ErrHelp
}

// run 根据第一个参数分发子命令。
func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "calendar":
		return runCalendar(args[1:])
	case "contacts":
		return runContacts(args[1:])
	case "auth":
		return runAuth(args[1:])
	case "account":
		return runAccount(args[1:])
	case "mail":
		return runMail(args[1:])
	case "mailbox":
		return runMailbox(args[1:])
	case "audit":
		return runAudit(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "update":
		return runUpdate(args[1:])
	case "completion":
		return runCompletion(args[1:])
	case "ui":
		return runUI(args[1:])
	case "config":
		return runConfig(args[1:])
	case "sync":
		return runSync(args[1:])
	case "search", "local-search":
		return runLocalSearch(args[1:])
	case "version", "--version", "-v":
		fmt.Fprintf(os.Stdout, "outlookctl %s\n", version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("未知命令: %s", args[0])
	}
}

// printGroupUsage 输出一级子命令的帮助。
func printGroupUsage(group string) {
	switch group {
	case "auth":
		fmt.Fprintln(os.Stdout, "用法: outlookctl auth <login|status>")
	case "account":
		fmt.Fprintln(os.Stdout, "用法: outlookctl account <list|use>")
	case "mail":
		fmt.Fprintln(os.Stdout, "用法: outlookctl mail <子命令> [options]")
		fmt.Fprintln(os.Stdout, "子命令: list, search, query, view, send, read, mark-read, mark-unread, folders, move, delete, permanent-delete, attachments, attachment-save, export-eml, import-eml, reply, reply-all, forward, draft-create, drafts, draft-send, draft-update, draft-attachment-delete, draft-attachment-replace, draft-attachment-bulk-delete, bulk-move, bulk-delete, bulk-mark-read, bulk-mark-unread, bulk-flag, bulk-unflag, bulk-importance, bulk-category-add, bulk-category-remove, folder-tree, folder-create, folder-rename, folder-delete, flag, unflag, importance, message-categories, category-add, category-remove, rules, rule-create, rule-update, rule-apply, rule-delete, master-categories, master-category-create, master-category-update, master-category-delete")
	case "calendar":
		fmt.Fprintln(os.Stdout, "用法: outlookctl calendar <子命令> [options]")
		fmt.Fprintln(os.Stdout, "子命令: list, calendars, attachments, attachment-add, attachment-get, attachment-delete, attachment-replace, attachment-bulk-delete, export-ics, import-ics, series-info, series-update, series-delete, freebusy, find-time, create, update, delete, cancel, respond")
	case "contacts":
		fmt.Fprintln(os.Stdout, "用法: outlookctl contacts <子命令> [options]")
		fmt.Fprintln(os.Stdout, "子命令: list, search, folders, folder-create, folder-rename, folder-delete, create, update, delete, permanent-delete, photo-get, photo-set, move, copy, export, import, bulk-delete, duplicates")
	case "audit":
		fmt.Fprintln(os.Stdout, "用法: outlookctl audit <list|export|stats|verify|config|cleanup>")
	case "mailbox":
		fmt.Fprintln(os.Stdout, "用法: outlookctl mailbox <settings|auto-reply|timezone>")
		fmt.Fprintln(os.Stdout, "子命令: settings, auto-reply get/set, timezone get/set")
	}
}

// runAudit 处理本地审计日志子命令。
func runAudit(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		printGroupUsage("audit")
		return nil
	}
	if len(args) == 0 {
		printGroupUsage("audit")
		return errors.New("audit 需要指定子命令: list")
	}
	switch args[0] {
	case "list", "view":
		return runAuditList(args[1:])
	case "export":
		return runAuditExport(args[1:])
	case "stats":
		return runAuditStats(args[1:])
	case "verify":
		return runAuditVerify(args[1:])
	case "config":
		return runAuditConfig(args[1:])
	case "cleanup":
		return runAuditCleanup(args[1:])
	default:
		printGroupUsage("audit")
		return fmt.Errorf("未知 audit 子命令: %s", args[0])
	}
}

// runAuditList 查询本地审计日志。
func runAuditList(args []string) error {
	return runAuditListQuery(args)
}

// runAuth 处理 auth 子命令。
func runAuth(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		printGroupUsage("auth")
		return nil
	}
	if len(args) == 0 {
		printGroupUsage("auth")
		return errors.New("auth 需要指定子命令: login 或 status")
	}
	switch args[0] {
	case "login":
		return runAuthLogin()
	case "status":
		return runAuthStatus()
	default:
		printUsage()
		return fmt.Errorf("未知 auth 子命令: %s", args[0])
	}
}

// runAuthLogin 执行 Device Code 登录，并把账号档案保存到本地。
func runAuthLogin() error {
	// Device Code 通常有 15 分钟有效期，这里留出足够时间给用户完成浏览器授权。
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}

	fmt.Fprintln(os.Stdout, "开始 Device Code 登录...")
	result, err := client.LoginDeviceCode(ctx, os.Stdout)
	if err != nil {
		return err
	}

	record := store.AccountRecord{
		Username:             result.Account.Username,
		HomeAccountID:        result.Account.HomeAccountID,
		Environment:          result.Account.Environment,
		TenantID:             result.Account.TenantID,
		DisplayName:          result.Account.DisplayName,
		Scopes:               append([]string(nil), result.Account.Scopes...),
		LastLoginAt:          time.Now().UTC().Format(time.RFC3339),
		AccessTokenExpiresAt: result.Account.ExpiresAt.Format(time.RFC3339),
		RefreshTokenCached:   result.Account.RefreshTokenCached,
	}

	// 登录成功后再调用 /me 获取显示名；即使失败也不影响 token 已经持久化。
	me, meErr := graph.GetMe(ctx, result.AccessToken)
	if meErr != nil {
		fmt.Fprintf(os.Stderr, "警告: 登录成功，但读取 Microsoft Graph /me 失败: %v\n", meErr)
	} else {
		if me.DisplayName != "" {
			record.DisplayName = me.DisplayName
		}
		if me.UserPrincipalName != "" {
			record.Username = me.UserPrincipalName
		}
	}

	accounts, err := store.UpsertAccount(record)
	if err != nil {
		return fmt.Errorf("保存账号档案失败: %w", err)
	}

	fmt.Fprintln(os.Stdout, "登录成功")
	fmt.Fprintf(os.Stdout, "账号: %s\n", record.Username)
	if record.DisplayName != "" {
		fmt.Fprintf(os.Stdout, "显示名: %s\n", record.DisplayName)
	}
	fmt.Fprintf(os.Stdout, "默认账号: %s\n", accounts.DefaultAccount)
	return nil
}

// runAuthStatus 验证本地 token 缓存，并尝试为每个账号静默刷新 access token。
func runAuthStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}

	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		fmt.Fprintln(os.Stdout, "本地没有已登录账号。")
		return nil
	}

	accountsFile, err := store.LoadAccounts()
	if err != nil {
		return err
	}
	if accountsFile.DefaultAccount != "" {
		fmt.Fprintf(os.Stdout, "默认账号: %s\n", accountsFile.DefaultAccount)
	}

	for _, account := range accounts {
		fmt.Fprintf(os.Stdout, "- %s\n", account.Username)
		if account.HomeAccountID != "" {
			fmt.Fprintf(os.Stdout, "  homeAccountId: %s\n", account.HomeAccountID)
		}
		if account.TenantID != "" {
			fmt.Fprintf(os.Stdout, "  tenantId: %s\n", account.TenantID)
		}

		result, err := client.AcquireTokenSilent(ctx, account)
		if err != nil {
			fmt.Fprintf(os.Stdout, "  静默刷新: 失败: %v\n", err)
			continue
		}
		fmt.Fprintf(os.Stdout, "  静默刷新: 成功，access token 过期时间: %s\n", result.Account.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

// runAccount 处理 account 子命令。
func runAccount(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		printGroupUsage("account")
		return nil
	}
	if len(args) == 0 {
		printGroupUsage("account")
		return errors.New("account 需要指定子命令: list 或 use")
	}
	switch args[0] {
	case "list":
		return runAccountList()
	case "use":
		return runAccountUse(args[1:])
	default:
		printUsage()
		return fmt.Errorf("未知 account 子命令: %s", args[0])
	}
}

// runAccountList 输出 accounts.json 中的可读账号档案。
func runAccountList() error {
	accounts, err := store.LoadAccounts()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化账号档案失败: %w", err)
	}
	fmt.Fprintln(os.Stdout, string(data))
	return nil
}

// runAccountUse 切换默认账号。
func runAccountUse(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl account use <email>")
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl account use <email>")
	}
	target := strings.TrimSpace(args[0])
	if target == "" {
		return errors.New("账号邮箱不能为空")
	}

	accounts, err := store.LoadAccounts()
	if err != nil {
		return err
	}
	for i := range accounts.Accounts {
		account := accounts.Accounts[i]
		if strings.EqualFold(account.Username, target) || account.HomeAccountID == target {
			accounts.DefaultAccount = account.Username
			if err := store.SaveAccounts(accounts); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "默认账号已切换为: %s\n", account.Username)
			return nil
		}
	}
	return fmt.Errorf("找不到账号: %s", target)
}

// runMail 处理 mail 子命令。
func runMail(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		printGroupUsage("mail")
		return nil
	}
	if len(args) == 0 {
		printGroupUsage("mail")
		return errors.New("mail 需要指定子命令: list")
	}
	switch args[0] {
	case "list":
		return runMailList(args[1:])
	case "search":
		return runMailSearch(args[1:])
	case "query":
		return runMailQuery(args[1:])
	case "view":
		return runMailView(args[1:])
	case "send":
		return runMailSend(args[1:])
	case "read":
		return runMailRead(args[1:])
	case "mark-read":
		return runMailSetRead(args[1:], true)
	case "mark-unread":
		return runMailSetRead(args[1:], false)
	case "folders":
		return runMailFolders(args[1:])
	case "folder-tree":
		return runMailFolderTree(args[1:])
	case "move":
		return runMailMove(args[1:])
	case "delete":
		return runMailDelete(args[1:])
	case "permanent-delete":
		return runMailPermanentDelete(args[1:])
	case "attachments":
		return runMailAttachments(args[1:])
	case "attachment-save":
		return runMailAttachmentSave(args[1:])
	case "export-eml":
		return runMailExportEML(args[1:])
	case "import-eml":
		return runMailImportEML(args[1:])
	case "reply":
		return runMailReply(args[1:], false)
	case "reply-all":
		return runMailReply(args[1:], true)
	case "forward":
		return runMailForward(args[1:])
	case "draft-create":
		return runMailDraftCreate(args[1:])
	case "drafts":
		return runMailDrafts(args[1:])
	case "draft-send":
		return runMailDraftSend(args[1:])
	case "draft-update":
		return runMailDraftUpdate(args[1:])
	case "draft-attachment-delete":
		return runMailDraftAttachmentDelete(args[1:])
	case "draft-attachment-replace":
		return runMailDraftAttachmentReplace(args[1:])
	case "draft-attachment-bulk-delete":
		return runMailDraftAttachmentBulkDelete(args[1:])
	case "bulk-mark-read":
		return runMailBulkMarkRead(args[1:], true)
	case "bulk-mark-unread":
		return runMailBulkMarkRead(args[1:], false)
	case "bulk-flag":
		return runMailBulkFlag(args[1:], true)
	case "bulk-unflag":
		return runMailBulkFlag(args[1:], false)
	case "bulk-importance":
		return runMailBulkImportance(args[1:])
	case "bulk-category-add":
		return runMailBulkCategory(args[1:], true)
	case "bulk-category-remove":
		return runMailBulkCategory(args[1:], false)
	case "bulk-move":
		return runMailBulkMove(args[1:])
	case "bulk-delete":
		return runMailBulkDelete(args[1:])
	case "folder-create":
		return runMailFolderCreate(args[1:])
	case "folder-rename":
		return runMailFolderRename(args[1:])
	case "folder-delete":
		return runMailFolderDelete(args[1:])
	case "flag":
		return runMailSetFlag(args[1:], true)
	case "unflag":
		return runMailSetFlag(args[1:], false)
	case "importance":
		return runMailSetImportance(args[1:])
	case "message-categories":
		return runMailMessageCategories(args[1:])
	case "category-add":
		return runMailCategoryAdd(args[1:])
	case "category-remove":
		return runMailCategoryRemove(args[1:])
	case "rules":
		return runMailRules(args[1:])
	case "rule-create":
		return runMailRuleCreate(args[1:])
	case "rule-update":
		return runMailRuleUpdate(args[1:])
	case "rule-apply":
		return runMailRuleApply(args[1:])
	case "rule-delete":
		return runMailRuleDelete(args[1:])
	case "master-categories":
		return runMailMasterCategories(args[1:])
	case "master-category-create":
		return runMailMasterCategoryCreate(args[1:])
	case "master-category-update":
		return runMailMasterCategoryUpdate(args[1:])
	case "master-category-delete":
		return runMailMasterCategoryDelete(args[1:])
	default:
		printUsage()
		return fmt.Errorf("未知 mail 子命令: %s", args[0])
	}
}

// runMailList 读取当前账号收件箱或指定文件夹中的最新邮件。
func runMailList(args []string) error {
	fs := flag.NewFlagSet("mail list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	folderFlag := fs.String("folder", "", "指定文件夹名称或 ID；不填时使用收件箱")
	topFlag := fs.Int("top", 10, "最多返回多少封邮件，范围 1-100")
	allFlag := fs.Bool("all", false, "分页拉取多页邮件")
	maxFlag := fs.Int("max", 500, "全量拉取时的最大邮件数量，范围 1-1000")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	outputFlag := fs.String("output", "", "输出格式: text/table/json")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	format, formatErr := effectiveOutputFormat(*outputFlag, *jsonFlag)
	if formatErr != nil {
		return formatErr
	}
	if format == "json" {
		*jsonFlag = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}

	var messages []graph.Message
	if *allFlag {
		folderID := "inbox"
		if strings.TrimSpace(*folderFlag) != "" {
			resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, *folderFlag)
			if resolveErr != nil {
				return resolveErr
			}
			folderID = resolved
		}
		messages, err = graph.ListMessagesInFolderAll(ctx, loginResult.AccessToken, folderID, *maxFlag)
	} else if strings.TrimSpace(*folderFlag) == "" {
		messages, err = graph.ListInboxMessages(ctx, loginResult.AccessToken, *topFlag)
	} else {
		folderID, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, *folderFlag)
		if resolveErr != nil {
			return resolveErr
		}
		messages, err = graph.ListMessagesInFolder(ctx, loginResult.AccessToken, folderID, *topFlag)
	}
	if err != nil {
		return err
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化邮件列表失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	if format == "table" {
		return printMailTable(messages, selected.Username)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	if strings.TrimSpace(*folderFlag) != "" {
		fmt.Fprintf(os.Stdout, "文件夹: %s\n", *folderFlag)
	}
	fmt.Fprintf(os.Stdout, "邮件数量: %d\n", len(messages))
	for i, message := range messages {
		readState := "未读"
		if message.IsRead {
			readState = "已读"
		}
		sender := message.From.EmailAddress.Address
		if sender == "" {
			sender = message.From.EmailAddress.Name
		}
		fmt.Fprintf(
			os.Stdout,
			"%2d. [%s] %s | %s | %s\n",
			i+1,
			message.ReceivedDateTime.Local().Format("2006-01-02 15:04"),
			readState,
			sender,
			message.Subject,
		)
	}
	return nil
}

// runMailRead 读取单封邮件详情。
// messageTarget 表示需要指定邮件 ID 和可选账号的命令参数。
type messageTarget struct {
	MessageID string
	Account   string
	JSON      bool
}

// parseMessageTargetArgs 手动解析邮件 ID、账号和 JSON 参数。
// 标准库 flag 在遇到第一个非 flag 参数后会停止解析，因此这里手写解析，
// 让 --json、--account 可以出现在邮件 ID 前后任意位置。
func parseMessageTargetArgs(args []string, allowJSON bool) (messageTarget, error) {
	var result messageTarget
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json" && allowJSON:
			result.JSON = true
		case arg == "--account":
			if i+1 >= len(args) {
				return result, errors.New("--account 缺少值")
			}
			i++
			result.Account = args[i]
		case strings.HasPrefix(arg, "--account="):
			result.Account = strings.TrimPrefix(arg, "--account=")
		case strings.HasPrefix(arg, "-"):
			return result, fmt.Errorf("未知参数: %s", arg)
		default:
			if result.MessageID != "" {
				return result, errors.New("只能指定一个邮件 ID")
			}
			result.MessageID = arg
		}
	}
	if result.MessageID == "" {
		return result, errors.New("缺少邮件 ID")
	}
	return result, nil
}

// runMailRead 读取单封邮件详情。
func runMailRead(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail read <message-id> [--json] [--account <email>]")
	}
	target, err := parseMessageTargetArgs(args, true)
	if err != nil {
		return errors.New("用法: outlookctl mail read <message-id> [--json] [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	detail, err := graph.GetMessage(ctx, loginResult.AccessToken, target.MessageID)
	if err != nil {
		return err
	}

	if target.JSON {
		data, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化邮件详情失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "主题: %s\n", detail.Subject)
	if detail.From.EmailAddress.Address != "" {
		fmt.Fprintf(os.Stdout, "发件人: %s <%s>\n", detail.From.EmailAddress.Name, detail.From.EmailAddress.Address)
	}
	fmt.Fprintf(os.Stdout, "收件时间: %s\n", detail.ReceivedDateTime.Local().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(os.Stdout, "已读: %v\n", detail.IsRead)
	if detail.WebLink != "" {
		fmt.Fprintf(os.Stdout, "链接: %s\n", detail.WebLink)
	}
	fmt.Fprintln(os.Stdout, "正文:")
	fmt.Fprintln(os.Stdout, detail.Body.Content)
	return nil
}

// runMailSearch 在全部邮箱文件夹或指定文件夹中搜索邮件。
func runMailSearch(args []string) error {
	fs := flag.NewFlagSet("mail search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	queryFlag := fs.String("query", "", "搜索关键字")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	folderFlag := fs.String("folder", "", "指定文件夹名称或 ID；不填时搜索全部文件夹")
	topFlag := fs.Int("top", 10, "最多返回多少封邮件，范围 1-100")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*queryFlag) == "" {
		return errors.New("用法: outlookctl mail search --query <关键字>")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}

	var messages []graph.Message
	if strings.TrimSpace(*folderFlag) == "" {
		messages, err = graph.SearchMessages(ctx, loginResult.AccessToken, *queryFlag, *topFlag)
	} else {
		folderID, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, *folderFlag)
		if resolveErr != nil {
			return resolveErr
		}
		messages, err = graph.SearchMessagesInFolder(ctx, loginResult.AccessToken, folderID, *queryFlag, *topFlag)
	}
	if err != nil {
		return err
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(messages, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化搜索结果失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "搜索关键字: %s\n", *queryFlag)
	fmt.Fprintf(os.Stdout, "结果数量: %d\n", len(messages))
	for i, message := range messages {
		readState := "未读"
		if message.IsRead {
			readState = "已读"
		}
		sender := message.From.EmailAddress.Address
		if sender == "" {
			sender = message.From.EmailAddress.Name
		}
		fmt.Fprintf(
			os.Stdout,
			"%2d. [%s] %s | %s | %s\n",
			i+1,
			message.ReceivedDateTime.Local().Format("2006-01-02 15:04"),
			readState,
			sender,
			message.Subject,
		)
	}
	return nil
}

// runMailSend 发送一封邮件。
func runMailSend(args []string) error {
	fs := flag.NewFlagSet("mail send", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	toFlag := fs.String("to", "", "收件人，多个地址用逗号分隔")
	ccFlag := fs.String("cc", "", "抄送，多个地址用逗号分隔")
	bccFlag := fs.String("bcc", "", "密送，多个地址用逗号分隔")
	subjectFlag := fs.String("subject", "", "邮件主题")
	bodyFlag := fs.String("body", "", "邮件正文")
	bodyFileFlag := fs.String("body-file", "", "从文件读取邮件正文")
	attachFlag := fs.String("attach", "", "附件路径，多个用逗号分隔")
	htmlFlag := fs.Bool("html", false, "正文使用 HTML 格式")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	noSaveFlag := fs.Bool("no-save", false, "不保存到已发送邮件")
	importanceFlag := fs.String("importance", "", "重要性: low/normal/high")
	readReceiptFlag := fs.Bool("read-receipt", false, "请求已读回执")
	deliveryReceiptFlag := fs.Bool("delivery-receipt", false, "请求送达回执")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}

	to := splitAddresses(*toFlag)
	if len(to) == 0 {
		return errors.New("用法: outlookctl mail send --to <邮箱> --subject <主题> --body <正文>")
	}

	body := *bodyFlag
	if strings.TrimSpace(*bodyFileFlag) != "" {
		data, err := os.ReadFile(*bodyFileFlag)
		if err != nil {
			return fmt.Errorf("读取正文文件失败: %w", err)
		}
		body = string(data)
	}

	attachments, err := graph.LoadMailAttachments(splitAddresses(*attachFlag))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}

	contentType := "Text"
	if *htmlFlag {
		contentType = "HTML"
	}
	input := graph.SendMailInput{
		To:                     to,
		Cc:                     splitAddresses(*ccFlag),
		Bcc:                    splitAddresses(*bccFlag),
		Subject:                *subjectFlag,
		Body:                   body,
		ContentType:            contentType,
		SaveToSentItems:        !*noSaveFlag,
		Attachments:            attachments,
		Importance:             *importanceFlag,
		RequestReadReceipt:     *readReceiptFlag,
		RequestDeliveryReceipt: *deliveryReceiptFlag,
	}
	if err := graph.SendMail(ctx, loginResult.AccessToken, input); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件已发送\n")
	fmt.Fprintf(os.Stdout, "收件人: %s\n", strings.Join(to, ", "))
	fmt.Fprintf(os.Stdout, "主题: %s\n", *subjectFlag)
	return nil
}

// splitAddresses 按英文逗号拆分多个邮箱地址，并去掉空白。
func splitAddresses(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// runMailSetRead 修改单封邮件的已读状态。
func runMailSetRead(args []string, isRead bool) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail mark-read|mark-unread <message-id> [--account <email>]")
	}
	target, err := parseMessageTargetArgs(args, false)
	if err != nil {
		return errors.New("用法: outlookctl mail mark-read|mark-unread <message-id> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.SetMessageRead(ctx, loginResult.AccessToken, target.MessageID, isRead); err != nil {
		return err
	}

	state := "已读"
	if !isRead {
		state = "未读"
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件已标记为%s: %s\n", state, target.MessageID)
	return nil
}

// runMailFolders 列出当前账号的邮件文件夹。
func runMailFolders(args []string) error {
	fs := flag.NewFlagSet("mail folders", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	topFlag := fs.Int("top", 50, "最多返回多少个文件夹，范围 1-200")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folders, err := graph.ListMailFolders(ctx, loginResult.AccessToken, *topFlag)
	if err != nil {
		return err
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(folders, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化邮件文件夹失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件文件夹数量: %d\n", len(folders))
	for _, folder := range folders {
		fmt.Fprintf(
			os.Stdout,
			"- %s | id=%s | 未读=%d | 总数=%d\n",
			folder.DisplayName,
			folder.ID,
			folder.UnreadItemCount,
			folder.TotalItemCount,
		)
	}
	return nil
}

// runMailMove 把邮件移动到指定文件夹。
func runMailMove(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail move <message-id> --folder <文件夹> [--account <email>]")
	}
	var account, folder, messageID string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) {
				return errors.New("--account 缺少值")
			}
			i++
			account = args[i]
		case strings.HasPrefix(arg, "--account="):
			account = strings.TrimPrefix(arg, "--account=")
		case arg == "--folder":
			if i+1 >= len(args) {
				return errors.New("--folder 缺少值")
			}
			i++
			folder = args[i]
		case strings.HasPrefix(arg, "--folder="):
			folder = strings.TrimPrefix(arg, "--folder=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("未知参数: %s", arg)
		default:
			if messageID != "" {
				return errors.New("只能指定一个邮件 ID")
			}
			messageID = arg
		}
	}
	if messageID == "" || folder == "" {
		return errors.New("用法: outlookctl mail move <message-id> --folder <文件夹> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	destinationID, err := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, folder)
	if err != nil {
		return err
	}
	newMessageID, err := graph.MoveMessage(ctx, loginResult.AccessToken, messageID, destinationID)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件已移动\n")
	fmt.Fprintf(os.Stdout, "原邮件 ID: %s\n", messageID)
	fmt.Fprintf(os.Stdout, "新邮件 ID: %s\n", newMessageID)
	return nil
}

// runMailDelete 删除邮件。
// 当前使用 Graph 默认删除行为：邮件会被移动到已删除邮件文件夹。
func runMailDelete(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail delete <message-id> [--account <email>]")
	}
	target, err := parseMessageTargetArgs(args, false)
	if err != nil {
		return errors.New("用法: outlookctl mail delete <message-id> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteMessage(ctx, loginResult.AccessToken, target.MessageID); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件已删除（已移动到已删除邮件）: %s\n", target.MessageID)
	return nil
}

// runMailAttachments 列出指定邮件的附件。
func runMailAttachments(args []string) error {
	target, err := parseMessageTargetArgs(args, true)
	if err != nil {
		return errors.New("用法: outlookctl mail attachments <message-id> [--json] [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	attachments, err := graph.ListAttachments(ctx, loginResult.AccessToken, target.MessageID)
	if err != nil {
		return err
	}

	if target.JSON {
		data, err := json.MarshalIndent(attachments, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化附件列表失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "附件数量: %d\n", len(attachments))
	for _, attachment := range attachments {
		inline := "否"
		if attachment.IsInline {
			inline = "是"
		}
		fmt.Fprintf(
			os.Stdout,
			"- %s | id=%s | 类型=%s | 大小=%d | 内嵌=%s\n",
			attachment.Name,
			attachment.ID,
			attachment.ContentType,
			attachment.Size,
			inline,
		)
	}
	return nil
}

// runMailAttachmentSave 下载并保存指定附件。
func runMailAttachmentSave(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail attachment-save <message-id> --attachment <id> --out <path> [--account <email>]")
	}
	var account, attachmentID, outputPath, messageID string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) {
				return errors.New("--account 缺少值")
			}
			i++
			account = args[i]
		case strings.HasPrefix(arg, "--account="):
			account = strings.TrimPrefix(arg, "--account=")
		case arg == "--attachment":
			if i+1 >= len(args) {
				return errors.New("--attachment 缺少值")
			}
			i++
			attachmentID = args[i]
		case strings.HasPrefix(arg, "--attachment="):
			attachmentID = strings.TrimPrefix(arg, "--attachment=")
		case arg == "--out":
			if i+1 >= len(args) {
				return errors.New("--out 缺少值")
			}
			i++
			outputPath = args[i]
		case strings.HasPrefix(arg, "--out="):
			outputPath = strings.TrimPrefix(arg, "--out=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("未知参数: %s", arg)
		default:
			if messageID != "" {
				return errors.New("只能指定一个邮件 ID")
			}
			messageID = arg
		}
	}
	if messageID == "" || attachmentID == "" || outputPath == "" {
		return errors.New("用法: outlookctl mail attachment-save <message-id> --attachment <id> --out <path> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	size, err := graph.SaveAttachment(ctx, loginResult.AccessToken, messageID, attachmentID, outputPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "附件已保存: %s\n", outputPath)
	fmt.Fprintf(os.Stdout, "大小: %d 字节\n", size)
	return nil
}

// runMailReply 回复或回复全部邮件。
func runMailReply(args []string, replyAll bool) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail reply|reply-all <message-id> --body <正文> [--account <email>]")
	}
	var account, body, messageID string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) {
				return errors.New("--account 缺少值")
			}
			i++
			account = args[i]
		case strings.HasPrefix(arg, "--account="):
			account = strings.TrimPrefix(arg, "--account=")
		case arg == "--body":
			if i+1 >= len(args) {
				return errors.New("--body 缺少值")
			}
			i++
			body = args[i]
		case strings.HasPrefix(arg, "--body="):
			body = strings.TrimPrefix(arg, "--body=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("未知参数: %s", arg)
		default:
			if messageID != "" {
				return errors.New("只能指定一个邮件 ID")
			}
			messageID = arg
		}
	}
	if messageID == "" {
		return errors.New("用法: outlookctl mail reply|reply-all <message-id> --body <正文> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if replyAll {
		err = graph.ReplyAllMessage(ctx, loginResult.AccessToken, messageID, body)
	} else {
		err = graph.ReplyMessage(ctx, loginResult.AccessToken, messageID, body)
	}
	if err != nil {
		return err
	}

	action := "回复"
	if replyAll {
		action = "回复全部"
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件已%s: %s\n", action, messageID)
	return nil
}

// runMailForward 转发邮件。
func runMailForward(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail forward <message-id> --to <邮箱> [--body <正文>] [--account <email>]")
	}
	var account, body, messageID string
	var to []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) {
				return errors.New("--account 缺少值")
			}
			i++
			account = args[i]
		case strings.HasPrefix(arg, "--account="):
			account = strings.TrimPrefix(arg, "--account=")
		case arg == "--body":
			if i+1 >= len(args) {
				return errors.New("--body 缺少值")
			}
			i++
			body = args[i]
		case strings.HasPrefix(arg, "--body="):
			body = strings.TrimPrefix(arg, "--body=")
		case arg == "--to":
			if i+1 >= len(args) {
				return errors.New("--to 缺少值")
			}
			i++
			to = append(to, splitAddresses(args[i])...)
		case strings.HasPrefix(arg, "--to="):
			to = append(to, splitAddresses(strings.TrimPrefix(arg, "--to="))...)
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("未知参数: %s", arg)
		default:
			if messageID != "" {
				return errors.New("只能指定一个邮件 ID")
			}
			messageID = arg
		}
	}
	if messageID == "" || len(to) == 0 {
		return errors.New("用法: outlookctl mail forward <message-id> --to <邮箱> [--body <正文>] [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.ForwardMessage(ctx, loginResult.AccessToken, messageID, body, to); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件已转发: %s\n", messageID)
	fmt.Fprintf(os.Stdout, "收件人: %s\n", strings.Join(to, ", "))
	return nil
}

// runMailDraftCreate 创建一封草稿邮件。
func runMailDraftCreate(args []string) error {
	fs := flag.NewFlagSet("mail draft-create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	toFlag := fs.String("to", "", "收件人，多个地址用逗号分隔")
	ccFlag := fs.String("cc", "", "抄送，多个地址用逗号分隔")
	bccFlag := fs.String("bcc", "", "密送，多个地址用逗号分隔")
	subjectFlag := fs.String("subject", "", "邮件主题")
	bodyFlag := fs.String("body", "", "邮件正文")
	attachFlag := fs.String("attach", "", "附件路径，多个用逗号分隔")
	htmlFlag := fs.Bool("html", false, "正文使用 HTML 格式")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	importanceFlag := fs.String("importance", "", "重要性: low/normal/high")
	readReceiptFlag := fs.Bool("read-receipt", false, "请求已读回执")
	deliveryReceiptFlag := fs.Bool("delivery-receipt", false, "请求送达回执")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}

	to := splitAddresses(*toFlag)
	if len(to) == 0 {
		return errors.New("用法: outlookctl mail draft-create --to <邮箱> --subject <主题> --body <正文>")
	}

	attachments, err := graph.LoadMailAttachments(splitAddresses(*attachFlag))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}

	contentType := "Text"
	if *htmlFlag {
		contentType = "HTML"
	}
	input := graph.SendMailInput{
		To:                     to,
		Cc:                     splitAddresses(*ccFlag),
		Bcc:                    splitAddresses(*bccFlag),
		Subject:                *subjectFlag,
		Body:                   *bodyFlag,
		ContentType:            contentType,
		Attachments:            attachments,
		Importance:             *importanceFlag,
		RequestReadReceipt:     *readReceiptFlag,
		RequestDeliveryReceipt: *deliveryReceiptFlag,
	}
	draftID, err := graph.CreateDraft(ctx, loginResult.AccessToken, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "草稿已创建: %s\n", draftID)
	return nil
}

// runMailDrafts 列出草稿箱中的邮件。
func runMailDrafts(args []string) error {
	fs := flag.NewFlagSet("mail drafts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	topFlag := fs.Int("top", 10, "最多返回多少封草稿，范围 1-100")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	drafts, err := graph.ListDrafts(ctx, loginResult.AccessToken, *topFlag)
	if err != nil {
		return err
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(drafts, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化草稿列表失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "草稿数量: %d\n", len(drafts))
	for i, draft := range drafts {
		fmt.Fprintf(os.Stdout, "%2d. [%s] %s\n", i+1, draft.ReceivedDateTime.Local().Format("2006-01-02 15:04"), draft.Subject)
	}
	return nil
}

// runMailDraftSend 发送指定草稿。
func runMailDraftSend(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail draft-send <draft-id> [--account <email>]")
	}
	target, err := parseMessageTargetArgs(args, false)
	if err != nil {
		return errors.New("用法: outlookctl mail draft-send <draft-id> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.SendDraft(ctx, loginResult.AccessToken, target.MessageID); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "草稿已发送: %s\n", target.MessageID)
	return nil
}

// runMailFolderCreate 创建邮件文件夹。
func runMailFolderCreate(args []string) error {
	fs := flag.NewFlagSet("mail folder-create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	nameFlag := fs.String("name", "", "文件夹名称")
	parentFlag := fs.String("parent", "", "父文件夹 ID，默认创建在根目录")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*nameFlag) == "" {
		return errors.New("用法: outlookctl mail folder-create --name <名称> [--parent <父文件夹ID>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folder, err := graph.CreateMailFolder(ctx, loginResult.AccessToken, *nameFlag, *parentFlag)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "文件夹已创建: %s\n", folder.DisplayName)
	fmt.Fprintf(os.Stdout, "文件夹 ID: %s\n", folder.ID)
	return nil
}

// runMailFolderRename 重命名邮件文件夹。
func runMailFolderRename(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail folder-rename <文件夹ID> --name <新名称> [--account <email>]")
	}
	var account, name, folderID string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) {
				return errors.New("--account 缺少值")
			}
			i++
			account = args[i]
		case strings.HasPrefix(arg, "--account="):
			account = strings.TrimPrefix(arg, "--account=")
		case arg == "--name":
			if i+1 >= len(args) {
				return errors.New("--name 缺少值")
			}
			i++
			name = args[i]
		case strings.HasPrefix(arg, "--name="):
			name = strings.TrimPrefix(arg, "--name=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("未知参数: %s", arg)
		default:
			if folderID != "" {
				return errors.New("只能指定一个文件夹 ID")
			}
			folderID = arg
		}
	}
	if folderID == "" || strings.TrimSpace(name) == "" {
		return errors.New("用法: outlookctl mail folder-rename <文件夹ID> --name <新名称> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.RenameMailFolder(ctx, loginResult.AccessToken, folderID, name); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "文件夹已重命名为: %s\n", name)
	return nil
}

// runMailFolderDelete 删除邮件文件夹。
func runMailFolderDelete(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail folder-delete <文件夹ID> [--account <email>]")
	}
	target, err := parseMessageTargetArgs(args, false)
	if err != nil {
		return errors.New("用法: outlookctl mail folder-delete <文件夹ID> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteMailFolder(ctx, loginResult.AccessToken, target.MessageID); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "文件夹已删除: %s\n", target.MessageID)
	return nil
}

// runMailSetFlag 设置或取消邮件旗标。
func runMailSetFlag(args []string, flagged bool) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail flag|unflag <message-id> [--account <email>]")
	}
	target, err := parseMessageTargetArgs(args, false)
	if err != nil {
		return errors.New("用法: outlookctl mail flag|unflag <message-id> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.SetMessageFlag(ctx, loginResult.AccessToken, target.MessageID, flagged); err != nil {
		return err
	}

	state := "已取消旗标"
	if flagged {
		state = "已设置旗标"
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件%s: %s\n", state, target.MessageID)
	return nil
}

// runMailSetImportance 设置邮件重要性。
func runMailSetImportance(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail importance <message-id> --level low|normal|high [--account <email>]")
	}
	var account, level, messageID string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) {
				return errors.New("--account 缺少值")
			}
			i++
			account = args[i]
		case strings.HasPrefix(arg, "--account="):
			account = strings.TrimPrefix(arg, "--account=")
		case arg == "--level":
			if i+1 >= len(args) {
				return errors.New("--level 缺少值")
			}
			i++
			level = args[i]
		case strings.HasPrefix(arg, "--level="):
			level = strings.TrimPrefix(arg, "--level=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("未知参数: %s", arg)
		default:
			if messageID != "" {
				return errors.New("只能指定一个邮件 ID")
			}
			messageID = arg
		}
	}
	if messageID == "" || level == "" {
		return errors.New("用法: outlookctl mail importance <message-id> --level low|normal|high [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.SetMessageImportance(ctx, loginResult.AccessToken, messageID, level); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件重要性已设置为 %s: %s\n", level, messageID)
	return nil
}

// runMailMessageCategories 列出单封邮件的类别。
func runMailMessageCategories(args []string) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail message-categories <message-id> [--json] [--account <email>]")
	}
	target, err := parseMessageTargetArgs(args, true)
	if err != nil {
		return errors.New("用法: outlookctl mail message-categories <message-id> [--json] [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, target.Account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	categories, err := graph.GetMessageCategories(ctx, loginResult.AccessToken, target.MessageID)
	if err != nil {
		return err
	}

	if target.JSON {
		data, err := json.MarshalIndent(categories, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化邮件类别失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件类别数量: %d\n", len(categories))
	for _, category := range categories {
		fmt.Fprintf(os.Stdout, "- %s\n", category)
	}
	return nil
}

// runMailCategoryAdd 给邮件添加类别。
func runMailCategoryAdd(args []string) error {
	return runMailCategoryChange(args, true)
}

// runMailCategoryRemove 从邮件移除类别。
func runMailCategoryRemove(args []string) error {
	return runMailCategoryChange(args, false)
}

// runMailCategoryChange 是添加或移除邮件类别的共用实现。
func runMailCategoryChange(args []string, add bool) error {
	if helpRequested(args) {
		return printHelpAndReturn("用法: outlookctl mail category-add|category-remove <message-id> --category <名称> [--account <email>]")
	}
	var account, category, messageID string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) {
				return errors.New("--account 缺少值")
			}
			i++
			account = args[i]
		case strings.HasPrefix(arg, "--account="):
			account = strings.TrimPrefix(arg, "--account=")
		case arg == "--category":
			if i+1 >= len(args) {
				return errors.New("--category 缺少值")
			}
			i++
			category = args[i]
		case strings.HasPrefix(arg, "--category="):
			category = strings.TrimPrefix(arg, "--category=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("未知参数: %s", arg)
		default:
			if messageID != "" {
				return errors.New("只能指定一个邮件 ID")
			}
			messageID = arg
		}
	}
	if messageID == "" || strings.TrimSpace(category) == "" {
		return errors.New("用法: outlookctl mail category-add|category-remove <message-id> --category <名称> [--account <email>]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, account)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	var categories []string
	if add {
		categories, err = graph.AddMessageCategory(ctx, loginResult.AccessToken, messageID, category)
	} else {
		categories, err = graph.RemoveMessageCategory(ctx, loginResult.AccessToken, messageID, category)
	}
	if err != nil {
		return err
	}

	action := "已添加类别"
	if !add {
		action = "已移除类别"
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件%s %s: %s\n", action, category, messageID)
	fmt.Fprintf(os.Stdout, "当前类别: %s\n", strings.Join(categories, ", "))
	return nil
}

// runCalendar 处理 calendar 子命令。
func runCalendar(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		printGroupUsage("calendar")
		return nil
	}
	if len(args) == 0 {
		printGroupUsage("calendar")
		return errors.New("calendar 需要指定子命令: list")
	}
	switch args[0] {
	case "list":
		return runCalendarList(args[1:])
	case "calendars":
		return runCalendarCalendars(args[1:])
	case "attachments":
		return runCalendarAttachments(args[1:])
	case "attachment-add":
		return runCalendarAttachmentAdd(args[1:])
	case "attachment-get":
		return runCalendarAttachmentGet(args[1:])
	case "attachment-delete":
		return runCalendarAttachmentDelete(args[1:])
	case "attachment-replace":
		return runCalendarAttachmentReplace(args[1:])
	case "attachment-bulk-delete":
		return runCalendarAttachmentBulkDelete(args[1:])
	case "export-ics":
		return runCalendarExportICS(args[1:])
	case "import-ics":
		return runCalendarImportICS(args[1:])
	case "series-info":
		return runCalendarSeriesInfo(args[1:])
	case "series-update":
		return runCalendarSeriesUpdate(args[1:])
	case "series-delete":
		return runCalendarSeriesDelete(args[1:])
	case "freebusy":
		return runCalendarFreeBusy(args[1:])
	case "find-time":
		return runCalendarFindTime(args[1:])
	case "create":
		return runCalendarCreate(args[1:])
	case "update":
		return runCalendarUpdate(args[1:])
	case "delete":
		return runCalendarDelete(args[1:])
	case "cancel":
		return runCalendarCancel(args[1:])
	case "respond":
		return runCalendarRespond(args[1:])
	case "forward":
		return runCalendarForward(args[1:])
	default:
		printUsage()
		return fmt.Errorf("未知 calendar 子命令: %s", args[0])
	}
}

// runCalendarList 读取指定天数范围内的日历事件。
func runCalendarList(args []string) error {
	fs := flag.NewFlagSet("calendar list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	calendarFlag := fs.String("calendar", "", "日历名称或 ID；不填时使用默认日历")
	daysFlag := fs.Int("days", 7, "查询未来多少天，范围 1-90")
	topFlag := fs.Int("top", 50, "最多返回多少条事件，范围 1-200")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	outputFlag := fs.String("output", "", "输出格式: text/table/json")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	format, formatErr := effectiveOutputFormat(*outputFlag, *jsonFlag)
	if formatErr != nil {
		return formatErr
	}
	if format == "json" {
		*jsonFlag = true
	}
	if *daysFlag <= 0 {
		*daysFlag = 7
	}
	if *daysFlag > 90 {
		*daysFlag = 90
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}

	start := time.Now().UTC()
	end := start.AddDate(0, 0, *daysFlag)
	calendarID, err := graph.ResolveCalendarID(ctx, loginResult.AccessToken, *calendarFlag)
	if err != nil {
		return err
	}
	events, err := graph.ListCalendarEventsInCalendar(ctx, loginResult.AccessToken, calendarID, start, end, *topFlag)
	if err != nil {
		return err
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化日历事件失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	if format == "table" {
		return printCalendarTable(events, selected.Username)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "事件数量: %d\n", len(events))
	for i, event := range events {
		fmt.Fprintf(
			os.Stdout,
			"%2d. [%s] %s | 地点=%s\n",
			i+1,
			event.Start.DateTime,
			event.Subject,
			event.Location.DisplayName,
		)
	}
	return nil
}

// runContacts 处理 contacts 子命令。
func runContacts(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		printGroupUsage("contacts")
		return nil
	}
	if len(args) == 0 {
		printGroupUsage("contacts")
		return errors.New("contacts 需要指定子命令: list")
	}
	switch args[0] {
	case "list":
		return runContactsList(args[1:])
	case "folders":
		return runContactsFolders(args[1:])
	case "folder-create":
		return runContactsFolderCreate(args[1:])
	case "folder-rename":
		return runContactsFolderRename(args[1:])
	case "folder-delete":
		return runContactsFolderDelete(args[1:])
	case "search":
		return runContactsSearch(args[1:])
	case "create":
		return runContactsCreate(args[1:])
	case "update":
		return runContactsUpdate(args[1:])
	case "delete":
		return runContactsDelete(args[1:])
	case "permanent-delete":
		return runContactsPermanentDelete(args[1:])
	case "photo-get":
		return runContactsPhotoGet(args[1:])
	case "photo-set":
		return runContactsPhotoSet(args[1:])
	case "move":
		return runContactsMove(args[1:])
	case "copy":
		return runContactsCopy(args[1:])
	case "export":
		return runContactsExport(args[1:])
	case "import":
		return runContactsImport(args[1:])
	case "bulk-delete":
		return runContactsBulkDelete(args[1:])
	case "duplicates":
		return runContactsDuplicates(args[1:])
	default:
		printUsage()
		return fmt.Errorf("未知 contacts 子命令: %s", args[0])
	}
}

// runContactsList 读取联系人列表。
func runContactsList(args []string) error {
	fs := flag.NewFlagSet("contacts list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	folderFlag := fs.String("folder", "", "联系人文件夹名称或 ID；不填时使用默认文件夹")
	topFlag := fs.Int("top", 50, "最多返回多少个联系人，范围 1-200")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	outputFlag := fs.String("output", "", "输出格式: text/table/json")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	format, formatErr := effectiveOutputFormat(*outputFlag, *jsonFlag)
	if formatErr != nil {
		return formatErr
	}
	if format == "json" {
		*jsonFlag = true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	var contacts []graph.Contact
	if strings.TrimSpace(*folderFlag) == "" {
		contacts, err = graph.ListContacts(ctx, loginResult.AccessToken, *topFlag)
	} else {
		folderID, resolveErr := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, *folderFlag)
		if resolveErr != nil {
			return resolveErr
		}
		contacts, err = graph.ListContactsInFolder(ctx, loginResult.AccessToken, folderID, *topFlag)
	}
	if err != nil {
		return err
	}

	if *jsonFlag {
		data, err := json.MarshalIndent(contacts, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化联系人失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}

	if format == "table" {
		return printContactsTable(contacts, selected.Username)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人数量: %d\n", len(contacts))
	for i, contact := range contacts {
		email := ""
		if len(contact.EmailAddresses) > 0 {
			email = contact.EmailAddresses[0].Address
		}
		fmt.Fprintf(os.Stdout, "%2d. %s | %s | %s\n", i+1, contact.DisplayName, email, contact.CompanyName)
	}
	return nil
}

// parseCalendarTime 解析命令行传入的日历时间。
func parseCalendarTime(value string) (time.Time, error) {
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

// runCalendarCreate 创建日历事件。
func runCalendarCreate(args []string) error {
	fs := flag.NewFlagSet("calendar create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	subjectFlag := fs.String("subject", "", "事件主题")
	startFlag := fs.String("start", "", "开始时间，例如 2026-10-20T10:00:00")
	endFlag := fs.String("end", "", "结束时间，例如 2026-10-20T11:00:00")
	bodyFlag := fs.String("body", "", "事件正文")
	locationFlag := fs.String("location", "", "事件地点")
	timezoneFlag := fs.String("timezone", "UTC", "时区，例如 UTC 或 China Standard Time")
	allDayFlag := fs.Bool("all-day", false, "是否为全天事件")
	attendeesFlag := fs.String("attendees", "", "参与人邮箱，多个用逗号分隔")
	reminderFlag := fs.Int("reminder-minutes", 0, "提前多少分钟提醒，0 表示使用服务端默认")
	repeatFlag := fs.String("repeat", "", "重复类型: daily/weekly/monthly/yearly")
	repeatIntervalFlag := fs.Int("repeat-interval", 1, "重复间隔")
	repeatCountFlag := fs.Int("repeat-count", 0, "重复次数")
	repeatUntilFlag := fs.String("repeat-until", "", "重复结束日期 YYYY-MM-DD")
	categoriesFlag := fs.String("categories", "", "事件类别，多个用逗号分隔")
	showAsFlag := fs.String("show-as", "", "日程状态: free/tentative/busy/oof/workingElsewhere/unknown")
	importanceFlag := fs.String("importance", "", "事件重要性: low/normal/high")
	sensitivityFlag := fs.String("sensitivity", "", "事件敏感度: normal/personal/private/confidential")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*subjectFlag) == "" || strings.TrimSpace(*startFlag) == "" || strings.TrimSpace(*endFlag) == "" {
		return errors.New("用法: outlookctl calendar create --subject <主题> --start <开始时间> --end <结束时间>")
	}
	start, err := parseCalendarTime(*startFlag)
	if err != nil {
		return err
	}
	end, err := parseCalendarTime(*endFlag)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	event, err := graph.CreateCalendarEvent(ctx, loginResult.AccessToken, graph.CalendarEventInput{
		Subject:                    *subjectFlag,
		Body:                       *bodyFlag,
		Start:                      start,
		End:                        end,
		TimeZone:                   *timezoneFlag,
		Location:                   *locationFlag,
		IsAllDay:                   *allDayFlag,
		Attendees:                  splitAddresses(*attendeesFlag),
		ReminderMinutesBeforeStart: *reminderFlag,
		RepeatType:                 *repeatFlag,
		RepeatInterval:             *repeatIntervalFlag,
		RepeatCount:                *repeatCountFlag,
		RepeatUntil:                *repeatUntilFlag,
		Categories:                 splitAddresses(*categoriesFlag),
		ShowAs:                     *showAsFlag,
		Importance:                 *importanceFlag,
		Sensitivity:                *sensitivityFlag,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "事件已创建: %s\n", event.Subject)
	fmt.Fprintf(os.Stdout, "事件 ID: %s\n", event.ID)
	return nil
}

// runCalendarUpdate 更新日历事件。
func runCalendarUpdate(args []string) error {
	fs := flag.NewFlagSet("calendar update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	subjectFlag := fs.String("subject", "", "事件主题")
	startFlag := fs.String("start", "", "开始时间")
	endFlag := fs.String("end", "", "结束时间")
	bodyFlag := fs.String("body", "", "事件正文")
	locationFlag := fs.String("location", "", "事件地点")
	timezoneFlag := fs.String("timezone", "UTC", "时区")
	allDayFlag := fs.Bool("all-day", false, "是否为全天事件")
	attendeesFlag := fs.String("attendees", "", "参与人邮箱，多个用逗号分隔")
	reminderFlag := fs.Int("reminder-minutes", 0, "提前多少分钟提醒，0 表示使用服务端默认")
	repeatFlag := fs.String("repeat", "", "重复类型: daily/weekly/monthly/yearly")
	repeatIntervalFlag := fs.Int("repeat-interval", 1, "重复间隔")
	repeatCountFlag := fs.Int("repeat-count", 0, "重复次数")
	repeatUntilFlag := fs.String("repeat-until", "", "重复结束日期 YYYY-MM-DD")
	categoriesFlag := fs.String("categories", "", "事件类别，多个用逗号分隔")
	showAsFlag := fs.String("show-as", "", "日程状态: free/tentative/busy/oof/workingElsewhere/unknown")
	importanceFlag := fs.String("importance", "", "事件重要性: low/normal/high")
	sensitivityFlag := fs.String("sensitivity", "", "事件敏感度: normal/personal/private/confidential")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*subjectFlag) == "" || strings.TrimSpace(*startFlag) == "" || strings.TrimSpace(*endFlag) == "" {
		return errors.New("用法: outlookctl calendar update --id <事件ID> --subject <主题> --start <开始时间> --end <结束时间>")
	}
	start, err := parseCalendarTime(*startFlag)
	if err != nil {
		return err
	}
	end, err := parseCalendarTime(*endFlag)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	event, err := graph.UpdateCalendarEvent(ctx, loginResult.AccessToken, *idFlag, graph.CalendarEventInput{
		Subject:                    *subjectFlag,
		Body:                       *bodyFlag,
		Start:                      start,
		End:                        end,
		TimeZone:                   *timezoneFlag,
		Location:                   *locationFlag,
		IsAllDay:                   *allDayFlag,
		Attendees:                  splitAddresses(*attendeesFlag),
		ReminderMinutesBeforeStart: *reminderFlag,
		RepeatType:                 *repeatFlag,
		RepeatInterval:             *repeatIntervalFlag,
		RepeatCount:                *repeatCountFlag,
		RepeatUntil:                *repeatUntilFlag,
		Categories:                 splitAddresses(*categoriesFlag),
		ShowAs:                     *showAsFlag,
		Importance:                 *importanceFlag,
		Sensitivity:                *sensitivityFlag,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "事件已更新: %s\n", event.Subject)
	fmt.Fprintf(os.Stdout, "事件 ID: %s\n", event.ID)
	return nil
}

// runCalendarDelete 删除日历事件。
func runCalendarDelete(args []string) error {
	fs := flag.NewFlagSet("calendar delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl calendar delete --id <事件ID>")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteCalendarEvent(ctx, loginResult.AccessToken, *idFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "事件已删除: %s\n", *idFlag)
	return nil
}

// runContactsCreate 创建联系人。
func runContactsCreate(args []string) error {
	fs := flag.NewFlagSet("contacts create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	folderFlag := fs.String("folder", "", "联系人文件夹名称或 ID；不填时使用默认文件夹")
	displayNameFlag := fs.String("display-name", "", "显示名称")
	givenNameFlag := fs.String("given-name", "", "名")
	surnameFlag := fs.String("surname", "", "姓")
	emailFlag := fs.String("email", "", "邮箱，多个用逗号分隔")
	companyFlag := fs.String("company", "", "公司")
	jobTitleFlag := fs.String("job-title", "", "职位")
	mobileFlag := fs.String("mobile", "", "手机号")
	businessFlag := fs.String("business-phone", "", "工作电话，多个用逗号分隔")
	nicknameFlag := fs.String("nickname", "", "昵称")
	birthdayFlag := fs.String("birthday", "", "生日，格式 YYYY-MM-DD")
	notesFlag := fs.String("notes", "", "个人备注")
	departmentFlag := fs.String("department", "", "部门")
	officeFlag := fs.String("office", "", "办公室位置")
	professionFlag := fs.String("profession", "", "职业")
	websiteFlag := fs.String("website", "", "网站，多个用逗号分隔")
	homeAddressFlag := fs.String("home-address", "", "家庭地址，格式 street|city|state|postal|country")
	businessAddressFlag := fs.String("business-address", "", "工作地址，格式 street|city|state|postal|country")
	otherAddressFlag := fs.String("other-address", "", "其他地址，格式 street|city|state|postal|country")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*displayNameFlag) == "" {
		return errors.New("用法: outlookctl contacts create --display-name <名称>")
	}
	input := graph.ContactInput{
		DisplayName:      *displayNameFlag,
		GivenName:        *givenNameFlag,
		Surname:          *surnameFlag,
		CompanyName:      *companyFlag,
		JobTitle:         *jobTitleFlag,
		MobilePhone:      *mobileFlag,
		NickName:         *nicknameFlag,
		Birthday:         parseBirthday(*birthdayFlag),
		PersonalNotes:    *notesFlag,
		Department:       *departmentFlag,
		OfficeLocation:   *officeFlag,
		Profession:       *professionFlag,
		BusinessHomePage: *websiteFlag,
		HomeAddress:      parseAddress(*homeAddressFlag),
		BusinessAddress:  parseAddress(*businessAddressFlag),
		OtherAddress:     parseAddress(*otherAddressFlag),
	}
	for _, address := range splitAddresses(*emailFlag) {
		input.EmailAddresses = append(input.EmailAddresses, graph.ContactEmailAddress{Address: address})
	}
	input.BusinessPhones = splitAddresses(*businessFlag)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	var contact *graph.Contact
	if strings.TrimSpace(*folderFlag) == "" {
		contact, err = graph.CreateContact(ctx, loginResult.AccessToken, input)
	} else {
		folderID, resolveErr := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, *folderFlag)
		if resolveErr != nil {
			return resolveErr
		}
		contact, err = graph.CreateContactInFolder(ctx, loginResult.AccessToken, folderID, input)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人已创建: %s\n", contact.DisplayName)
	fmt.Fprintf(os.Stdout, "联系人 ID: %s\n", contact.ID)
	return nil
}

// runContactsUpdate 更新联系人。
func runContactsUpdate(args []string) error {
	fs := flag.NewFlagSet("contacts update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "联系人 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	displayNameFlag := fs.String("display-name", "", "显示名称")
	givenNameFlag := fs.String("given-name", "", "名")
	surnameFlag := fs.String("surname", "", "姓")
	emailFlag := fs.String("email", "", "邮箱，多个用逗号分隔")
	companyFlag := fs.String("company", "", "公司")
	jobTitleFlag := fs.String("job-title", "", "职位")
	mobileFlag := fs.String("mobile", "", "手机号")
	businessFlag := fs.String("business-phone", "", "工作电话，多个用逗号分隔")
	nicknameFlag := fs.String("nickname", "", "昵称")
	birthdayFlag := fs.String("birthday", "", "生日，格式 YYYY-MM-DD")
	notesFlag := fs.String("notes", "", "个人备注")
	departmentFlag := fs.String("department", "", "部门")
	officeFlag := fs.String("office", "", "办公室位置")
	professionFlag := fs.String("profession", "", "职业")
	websiteFlag := fs.String("website", "", "网站，多个用逗号分隔")
	homeAddressFlag := fs.String("home-address", "", "家庭地址，格式 street|city|state|postal|country")
	businessAddressFlag := fs.String("business-address", "", "工作地址，格式 street|city|state|postal|country")
	otherAddressFlag := fs.String("other-address", "", "其他地址，格式 street|city|state|postal|country")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*displayNameFlag) == "" {
		return errors.New("用法: outlookctl contacts update --id <联系人ID> --display-name <名称>")
	}
	input := graph.ContactInput{
		DisplayName:      *displayNameFlag,
		GivenName:        *givenNameFlag,
		Surname:          *surnameFlag,
		CompanyName:      *companyFlag,
		JobTitle:         *jobTitleFlag,
		MobilePhone:      *mobileFlag,
		NickName:         *nicknameFlag,
		Birthday:         parseBirthday(*birthdayFlag),
		PersonalNotes:    *notesFlag,
		Department:       *departmentFlag,
		OfficeLocation:   *officeFlag,
		Profession:       *professionFlag,
		BusinessHomePage: *websiteFlag,
		HomeAddress:      parseAddress(*homeAddressFlag),
		BusinessAddress:  parseAddress(*businessAddressFlag),
		OtherAddress:     parseAddress(*otherAddressFlag),
	}
	for _, address := range splitAddresses(*emailFlag) {
		input.EmailAddresses = append(input.EmailAddresses, graph.ContactEmailAddress{Address: address})
	}
	input.BusinessPhones = splitAddresses(*businessFlag)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	contact, err := graph.UpdateContact(ctx, loginResult.AccessToken, *idFlag, input)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人已更新: %s\n", contact.DisplayName)
	return nil
}

// runContactsDelete 删除联系人。
func runContactsDelete(args []string) error {
	fs := flag.NewFlagSet("contacts delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "联系人 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl contacts delete --id <联系人ID>")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteContact(ctx, loginResult.AccessToken, *idFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人已删除: %s\n", *idFlag)
	return nil
}

// runContactsSearch 按显示名称前缀搜索联系人。
func runContactsSearch(args []string) error {
	fs := flag.NewFlagSet("contacts search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	queryFlag := fs.String("query", "", "搜索关键字，按显示名称前缀匹配")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	topFlag := fs.Int("top", 20, "最多返回多少个联系人，范围 1-200")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*queryFlag) == "" {
		return errors.New("用法: outlookctl contacts search --query <关键字>")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	contacts, err := graph.SearchContacts(ctx, loginResult.AccessToken, *queryFlag, *topFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(contacts, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化联系人搜索结果失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "搜索关键字: %s\n", *queryFlag)
	fmt.Fprintf(os.Stdout, "结果数量: %d\n", len(contacts))
	for i, contact := range contacts {
		email := ""
		if len(contact.EmailAddresses) > 0 {
			email = contact.EmailAddresses[0].Address
		}
		fmt.Fprintf(os.Stdout, "%2d. %s | %s | %s\n", i+1, contact.DisplayName, email, contact.CompanyName)
	}
	return nil
}

// runContactsFolders 列出联系人文件夹。
func runContactsFolders(args []string) error {
	fs := flag.NewFlagSet("contacts folders", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	topFlag := fs.Int("top", 50, "最多返回多少个文件夹，范围 1-200")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folders, err := graph.ListContactFolders(ctx, loginResult.AccessToken, *topFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(folders, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化联系人文件夹失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人文件夹数量: %d\n", len(folders))
	for _, folder := range folders {
		fmt.Fprintf(os.Stdout, "- %s | id=%s\n", folder.DisplayName, folder.ID)
	}
	return nil
}

// runContactsFolderCreate 创建联系人文件夹。
func runContactsFolderCreate(args []string) error {
	fs := flag.NewFlagSet("contacts folder-create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	nameFlag := fs.String("name", "", "文件夹名称")
	parentFlag := fs.String("parent", "", "父文件夹 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*nameFlag) == "" {
		return errors.New("用法: outlookctl contacts folder-create --name <名称>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folder, err := graph.CreateContactFolder(ctx, loginResult.AccessToken, *nameFlag, *parentFlag)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人文件夹已创建: %s\n", folder.DisplayName)
	fmt.Fprintf(os.Stdout, "文件夹 ID: %s\n", folder.ID)
	return nil
}

// runContactsFolderDelete 删除联系人文件夹。
func runContactsFolderDelete(args []string) error {
	fs := flag.NewFlagSet("contacts folder-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "文件夹 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl contacts folder-delete --id <文件夹ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteContactFolder(ctx, loginResult.AccessToken, *idFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人文件夹已删除: %s\n", *idFlag)
	return nil
}

// runMailRules 列收件箱规则。
func runMailRules(args []string) error {
	fs := flag.NewFlagSet("mail rules", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	rules, err := graph.ListMessageRules(ctx, loginResult.AccessToken)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(rules, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化收件箱规则失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "收件箱规则数量: %d\n", len(rules))
	for _, rule := range rules {
		fmt.Fprintf(os.Stdout, "- %s | enabled=%v | id=%s\n", rule.DisplayName, rule.IsEnabled, rule.ID)
	}
	return nil
}

// runMailRuleCreate 创建收件箱规则。
func runMailRuleCreate(args []string) error {
	fs := flag.NewFlagSet("mail rule-create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	nameFlag := fs.String("name", "", "规则名称")
	fromFlag := fs.String("from", "", "发件人邮箱，多个用逗号分隔")
	subjectFlag := fs.String("subject-contains", "", "主题包含关键字，多个用逗号分隔")
	moveFlag := fs.String("move-to", "", "移动到文件夹名称或 ID")
	markReadFlag := fs.Bool("mark-read", false, "标记为已读")
	deleteFlag := fs.Bool("delete", false, "删除邮件")
	stopFlag := fs.Bool("stop", false, "停止处理后续规则")
	categoryFlag := fs.String("category", "", "给邮件添加类别，多个用逗号分隔")
	bodyContainsFlag := fs.String("body-contains", "", "正文包含关键字，多个用逗号分隔")
	bodyOrSubjectFlag := fs.String("body-or-subject-contains", "", "正文或主题包含关键字，多个用逗号分隔")
	hasAttachmentsFlag := fs.Bool("has-attachments", false, "匹配带附件的邮件")
	ruleImportanceFlag := fs.String("importance", "", "匹配邮件重要性: low/normal/high")
	senderContainsFlag := fs.String("sender-contains", "", "发件人包含关键字，多个用逗号分隔")
	recipientContainsFlag := fs.String("recipient-contains", "", "收件人包含关键字，多个用逗号分隔")
	copyFlag := fs.String("copy-to", "", "复制到文件夹名称或 ID")
	forwardFlag := fs.String("forward-to", "", "转发到邮箱，多个用逗号分隔")
	redirectFlag := fs.String("redirect-to", "", "重定向到邮箱，多个用逗号分隔")
	markImportanceFlag := fs.String("mark-importance", "", "设置邮件重要性: low/normal/high")
	permanentDeleteFlag := fs.Bool("permanent-delete", false, "永久删除邮件")
	disabledFlag := fs.Bool("disabled", false, "创建为禁用状态")
	applyNowFlag := fs.Bool("apply-now", false, "创建后立即回放规则")
	confirmApplyFlag := fs.Bool("confirm-apply", false, "确认实际修改已有邮件；不填时只 dry-run")
	applyFolderFlag := fs.String("apply-folder", "", "回放规则的目标文件夹；不填时使用收件箱")
	applyFoldersFlag := fs.String("apply-folders", "", "回放规则的目标文件夹列表，多个用逗号分隔")
	applyMaxFlag := fs.Int("apply-max", 500, "回放时最多扫描多少封邮件")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*nameFlag) == "" {
		return errors.New("用法: outlookctl mail rule-create --name <名称> [--from <邮箱>] [--subject-contains <关键字>] [--move-to <文件夹>]")
	}
	from := splitAddresses(*fromFlag)
	subjects := splitAddresses(*subjectFlag)
	categories := splitAddresses(*categoryFlag)
	bodyContains := splitAddresses(*bodyContainsFlag)
	bodyOrSubjectContains := splitAddresses(*bodyOrSubjectFlag)
	senderContains := splitAddresses(*senderContainsFlag)
	recipientContains := splitAddresses(*recipientContainsFlag)
	forwardTo := splitAddresses(*forwardFlag)
	redirectTo := splitAddresses(*redirectFlag)
	if len(from) == 0 && len(subjects) == 0 && len(bodyContains) == 0 && len(bodyOrSubjectContains) == 0 && !*hasAttachmentsFlag && strings.TrimSpace(*ruleImportanceFlag) == "" && len(senderContains) == 0 && len(recipientContains) == 0 {
		return errors.New("至少需要 --from 或 --subject-contains 中的一个规则条件")
	}
	if strings.TrimSpace(*moveFlag) == "" && strings.TrimSpace(*copyFlag) == "" && len(categories) == 0 && len(forwardTo) == 0 && len(redirectTo) == 0 && !*markReadFlag && strings.TrimSpace(*markImportanceFlag) == "" && !*deleteFlag && !*permanentDeleteFlag && !*stopFlag {
		return errors.New("至少需要 --move-to、--mark-read、--delete 或 --stop 中的一个规则动作")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	moveToFolder := strings.TrimSpace(*moveFlag)
	if moveToFolder != "" {
		moveToFolder, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, moveToFolder)
		if err != nil {
			return err
		}
	}
	copyToFolder := strings.TrimSpace(*copyFlag)
	if copyToFolder != "" {
		copyToFolder, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, copyToFolder)
		if err != nil {
			return err
		}
	}
	rule, err := graph.CreateMessageRule(ctx, loginResult.AccessToken, graph.MessageRuleInput{
		DisplayName:           *nameFlag,
		FromAddresses:         from,
		SubjectContains:       subjects,
		MoveToFolder:          moveToFolder,
		MarkAsRead:            *markReadFlag,
		Delete:                *deleteFlag,
		AssignCategories:      categories,
		BodyContains:          bodyContains,
		BodyOrSubjectContains: bodyOrSubjectContains,
		HasAttachments:        *hasAttachmentsFlag,
		Importance:            *ruleImportanceFlag,
		SenderContains:        senderContains,
		RecipientContains:     recipientContains,
		CopyToFolder:          copyToFolder,
		ForwardTo:             forwardTo,
		RedirectTo:            redirectTo,
		MarkImportance:        *markImportanceFlag,
		PermanentDelete:       *permanentDeleteFlag,
		StopProcessingRules:   *stopFlag,
		IsEnabled:             !*disabledFlag,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "规则已创建: %s\n", rule.DisplayName)
	fmt.Fprintf(os.Stdout, "规则 ID: %s\n", rule.ID)
	if *applyNowFlag {
		folderIDs := make([]string, 0)
		if strings.TrimSpace(*applyFoldersFlag) != "" {
			for _, folder := range splitAddresses(*applyFoldersFlag) {
				resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, folder)
				if resolveErr != nil {
					return resolveErr
				}
				folderIDs = append(folderIDs, resolved)
			}
		} else {
			folderID := "inbox"
			if strings.TrimSpace(*applyFolderFlag) != "" {
				resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, *applyFolderFlag)
				if resolveErr != nil {
					return resolveErr
				}
				folderID = resolved
			}
			folderIDs = append(folderIDs, folderID)
		}
		applyResult, applyErr := graph.ApplyMessageRuleToFolders(ctx, loginResult.AccessToken, rule, folderIDs, *applyMaxFlag, *confirmApplyFlag)
		if applyErr != nil {
			return applyErr
		}
		printMessageRuleApplyResult(selected.Username, applyResult)
		if !*confirmApplyFlag {
			fmt.Fprintln(os.Stdout, "这是 dry-run，未修改任何邮件；确认后加 --confirm-apply 执行")
		}
	}
	return nil
}

// runMailRuleUpdate 更新收件箱规则。
func runMailRuleUpdate(args []string) error {
	fs := flag.NewFlagSet("mail rule-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	idFlag := fs.String("id", "", "规则 ID")
	nameFlag := fs.String("name", "", "规则名称")
	fromFlag := fs.String("from", "", "发件人邮箱，多个用逗号分隔")
	subjectFlag := fs.String("subject-contains", "", "主题包含关键字，多个用逗号分隔")
	moveFlag := fs.String("move-to", "", "移动到文件夹名称或 ID")
	markReadFlag := fs.Bool("mark-read", false, "标记为已读")
	deleteFlag := fs.Bool("delete", false, "删除邮件")
	stopFlag := fs.Bool("stop", false, "停止处理后续规则")
	categoryFlag := fs.String("category", "", "给邮件添加类别，多个用逗号分隔")
	bodyContainsFlag := fs.String("body-contains", "", "正文包含关键字，多个用逗号分隔")
	bodyOrSubjectFlag := fs.String("body-or-subject-contains", "", "正文或主题包含关键字，多个用逗号分隔")
	hasAttachmentsFlag := fs.Bool("has-attachments", false, "匹配带附件的邮件")
	ruleImportanceFlag := fs.String("importance", "", "匹配邮件重要性: low/normal/high")
	senderContainsFlag := fs.String("sender-contains", "", "发件人包含关键字，多个用逗号分隔")
	recipientContainsFlag := fs.String("recipient-contains", "", "收件人包含关键字，多个用逗号分隔")
	copyFlag := fs.String("copy-to", "", "复制到文件夹名称或 ID")
	forwardFlag := fs.String("forward-to", "", "转发到邮箱，多个用逗号分隔")
	redirectFlag := fs.String("redirect-to", "", "重定向到邮箱，多个用逗号分隔")
	markImportanceFlag := fs.String("mark-importance", "", "设置邮件重要性: low/normal/high")
	permanentDeleteFlag := fs.Bool("permanent-delete", false, "永久删除邮件")
	disabledFlag := fs.Bool("disabled", false, "更新为禁用状态")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*nameFlag) == "" {
		return errors.New("用法: outlookctl mail rule-update --id <规则ID> --name <名称> ...")
	}
	from := splitAddresses(*fromFlag)
	subjects := splitAddresses(*subjectFlag)
	categories := splitAddresses(*categoryFlag)
	bodyContains := splitAddresses(*bodyContainsFlag)
	bodyOrSubjectContains := splitAddresses(*bodyOrSubjectFlag)
	senderContains := splitAddresses(*senderContainsFlag)
	recipientContains := splitAddresses(*recipientContainsFlag)
	forwardTo := splitAddresses(*forwardFlag)
	redirectTo := splitAddresses(*redirectFlag)
	if len(from) == 0 && len(subjects) == 0 && len(bodyContains) == 0 && len(bodyOrSubjectContains) == 0 && !*hasAttachmentsFlag && strings.TrimSpace(*ruleImportanceFlag) == "" && len(senderContains) == 0 && len(recipientContains) == 0 {
		return errors.New("至少需要 --from 或 --subject-contains 中的一个规则条件")
	}
	if strings.TrimSpace(*moveFlag) == "" && strings.TrimSpace(*copyFlag) == "" && len(categories) == 0 && len(forwardTo) == 0 && len(redirectTo) == 0 && !*markReadFlag && strings.TrimSpace(*markImportanceFlag) == "" && !*deleteFlag && !*permanentDeleteFlag && !*stopFlag {
		return errors.New("至少需要 --move-to、--mark-read、--delete 或 --stop 中的一个规则动作")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	moveToFolder := strings.TrimSpace(*moveFlag)
	if moveToFolder != "" {
		moveToFolder, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, moveToFolder)
		if err != nil {
			return err
		}
	}
	copyToFolder := strings.TrimSpace(*copyFlag)
	if copyToFolder != "" {
		copyToFolder, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, copyToFolder)
		if err != nil {
			return err
		}
	}
	rule, err := graph.UpdateMessageRule(ctx, loginResult.AccessToken, *idFlag, graph.MessageRuleInput{
		DisplayName:           *nameFlag,
		FromAddresses:         from,
		SubjectContains:       subjects,
		MoveToFolder:          moveToFolder,
		MarkAsRead:            *markReadFlag,
		Delete:                *deleteFlag,
		AssignCategories:      categories,
		BodyContains:          bodyContains,
		BodyOrSubjectContains: bodyOrSubjectContains,
		HasAttachments:        *hasAttachmentsFlag,
		Importance:            *ruleImportanceFlag,
		SenderContains:        senderContains,
		RecipientContains:     recipientContains,
		CopyToFolder:          copyToFolder,
		ForwardTo:             forwardTo,
		RedirectTo:            redirectTo,
		MarkImportance:        *markImportanceFlag,
		PermanentDelete:       *permanentDeleteFlag,
		StopProcessingRules:   *stopFlag,
		IsEnabled:             !*disabledFlag,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "规则已更新: %s\n", rule.DisplayName)
	fmt.Fprintf(os.Stdout, "规则 ID: %s\n", rule.ID)
	return nil
}

// printMessageRuleApplyResult 输出规则回放结果。
// printMessageRuleApplyResult 输出规则回放结果。
func printMessageRuleApplyResult(username string, result *graph.RuleApplyResult) {
	folders := result.FolderIDs
	if len(folders) == 0 && result.FolderID != "" {
		folders = []string{result.FolderID}
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", username)
	fmt.Fprintf(os.Stdout, "规则: %s\n", result.RuleName)
	fmt.Fprintf(os.Stdout, "文件夹: %s\n", strings.Join(folders, ","))
	fmt.Fprintf(os.Stdout, "扫描邮件: %d\n", result.Scanned)
	fmt.Fprintf(os.Stdout, "匹配邮件: %d\n", result.Matched)
	fmt.Fprintf(os.Stdout, "已应用: %d\n", result.Applied)
	if result.DryRun {
		fmt.Fprintln(os.Stdout, "模式: dry-run，未修改任何邮件")
	} else {
		fmt.Fprintln(os.Stdout, "模式: 已实际应用")
	}
	for i, match := range result.Matches {
		fmt.Fprintf(os.Stdout, "%d. %s | %s | %s | %s\n", i+1, match.Subject, match.From, match.FolderID, strings.Join(match.Actions, ","))
	}
}

// runMailRuleApply 把已有收件箱规则回放到已有邮件。
// runMailRuleApply 把已有收件箱规则回放到已有邮件。
func runMailRuleApply(args []string) error {
	fs := flag.NewFlagSet("mail rule-apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	idFlag := fs.String("id", "", "规则 ID")
	folderFlag := fs.String("folder", "", "目标文件夹名称或 ID；不填时使用收件箱")
	foldersFlag := fs.String("folders", "", "目标文件夹列表，多个用逗号分隔")
	maxFlag := fs.Int("max", 500, "每个文件夹最多扫描多少封邮件，范围 1-1000")
	confirmFlag := fs.Bool("confirm", false, "确认实际修改邮件；不填时只 dry-run")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl mail rule-apply --id <规则ID> [--folder <文件夹>] [--folders <文件夹1,文件夹2>] [--max 500] [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	rule, err := graph.GetMessageRule(ctx, loginResult.AccessToken, *idFlag)
	if err != nil {
		return err
	}
	folderIDs := make([]string, 0)
	if strings.TrimSpace(*foldersFlag) != "" {
		for _, folder := range splitAddresses(*foldersFlag) {
			resolved, resolveErr := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, folder)
			if resolveErr != nil {
				return resolveErr
			}
			folderIDs = append(folderIDs, resolved)
		}
	} else {
		folderID := "inbox"
		if strings.TrimSpace(*folderFlag) != "" {
			folderID, err = graph.ResolveMailFolderID(ctx, loginResult.AccessToken, *folderFlag)
			if err != nil {
				return err
			}
		}
		folderIDs = append(folderIDs, folderID)
	}
	result, err := graph.ApplyMessageRuleToFolders(ctx, loginResult.AccessToken, rule, folderIDs, *maxFlag, *confirmFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化规则回放结果失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	printMessageRuleApplyResult(selected.Username, result)
	if *confirmFlag {
		fmt.Fprintln(os.Stdout, "规则回放完成")
	} else {
		fmt.Fprintln(os.Stdout, "这是 dry-run，未修改任何邮件；确认后加 --confirm 执行")
	}
	return nil
}

// runMailRuleDelete 删除收件箱规则。
func runMailRuleDelete(args []string) error {
	fs := flag.NewFlagSet("mail rule-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	idFlag := fs.String("id", "", "规则 ID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl mail rule-delete --id <规则ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteMessageRule(ctx, loginResult.AccessToken, *idFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "规则已删除: %s\n", *idFlag)
	return nil
}

// runMailMasterCategories 列出主类别。
func runMailMasterCategories(args []string) error {
	fs := flag.NewFlagSet("mail master-categories", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	categories, err := graph.ListMasterCategories(ctx, loginResult.AccessToken)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(categories, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化主类别失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "主类别数量: %d\n", len(categories))
	for _, category := range categories {
		fmt.Fprintf(os.Stdout, "- %s | color=%s | id=%s\n", category.DisplayName, category.Color, category.ID)
	}
	return nil
}

// runMailMasterCategoryCreate 创建主类别。
func runMailMasterCategoryCreate(args []string) error {
	fs := flag.NewFlagSet("mail master-category-create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	nameFlag := fs.String("name", "", "类别名称")
	colorFlag := fs.String("color", "", "类别颜色，例如 preset0")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*nameFlag) == "" {
		return errors.New("用法: outlookctl mail master-category-create --name <名称> [--color preset0]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	category, err := graph.CreateMasterCategory(ctx, loginResult.AccessToken, *nameFlag, *colorFlag)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "主类别已创建: %s\n", category.DisplayName)
	fmt.Fprintf(os.Stdout, "类别 ID: %s\n", category.ID)
	return nil
}

// runMailMasterCategoryUpdate 更新主类别。
func runMailMasterCategoryUpdate(args []string) error {
	fs := flag.NewFlagSet("mail master-category-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	idFlag := fs.String("id", "", "类别 ID")
	colorFlag := fs.String("color", "", "类别颜色，例如 preset0")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*colorFlag) == "" {
		return errors.New("用法: outlookctl mail master-category-update --id <类别ID> --color <颜色>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	category, err := graph.UpdateMasterCategory(ctx, loginResult.AccessToken, *idFlag, *colorFlag)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "主类别已更新: %s\n", category.DisplayName)
	fmt.Fprintf(os.Stdout, "类别 ID: %s\n", category.ID)
	return nil
}

// runMailMasterCategoryDelete 删除主类别。
func runMailMasterCategoryDelete(args []string) error {
	fs := flag.NewFlagSet("mail master-category-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	idFlag := fs.String("id", "", "类别 ID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl mail master-category-delete --id <类别ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteMasterCategory(ctx, loginResult.AccessToken, *idFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "主类别已删除: %s\n", *idFlag)
	return nil
}

// runContactsMove 把联系人移动到指定文件夹。
func runContactsMove(args []string) error {
	fs := flag.NewFlagSet("contacts move", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "联系人 ID")
	folderFlag := fs.String("folder", "", "目标文件夹名称或 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*folderFlag) == "" {
		return errors.New("用法: outlookctl contacts move --id <联系人ID> --folder <文件夹>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folderID, err := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, *folderFlag)
	if err != nil {
		return err
	}
	moved, err := graph.MoveContact(ctx, loginResult.AccessToken, *idFlag, folderID)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人已移动: %s\n", moved.DisplayName)
	fmt.Fprintf(os.Stdout, "新联系人 ID: %s\n", moved.ID)
	return nil
}

// runMailPermanentDelete 永久删除邮件。
func runMailPermanentDelete(args []string) error {
	fs := flag.NewFlagSet("mail permanent-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "邮件 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl mail permanent-delete --id <邮件ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.PermanentDeleteMessage(ctx, loginResult.AccessToken, *idFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "邮件已永久删除: %s\n", *idFlag)
	return nil
}

// runContactsCopy 复制联系人到指定文件夹。
func runContactsCopy(args []string) error {
	fs := flag.NewFlagSet("contacts copy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "联系人 ID")
	folderFlag := fs.String("folder", "", "目标文件夹名称或 ID")
	nameFlag := fs.String("name", "", "新联系人显示名称，不填时沿用原名")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*folderFlag) == "" {
		return errors.New("用法: outlookctl contacts copy --id <联系人ID> --folder <文件夹> [--name <新名称>]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folderID, err := graph.ResolveContactFolderID(ctx, loginResult.AccessToken, *folderFlag)
	if err != nil {
		return err
	}
	copied, err := graph.CopyContact(ctx, loginResult.AccessToken, *idFlag, folderID, *nameFlag)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人已复制: %s\n", copied.DisplayName)
	fmt.Fprintf(os.Stdout, "新联系人 ID: %s\n", copied.ID)
	return nil
}

// runCalendarCalendars 列出日历。
func runCalendarCalendars(args []string) error {
	fs := flag.NewFlagSet("calendar calendars", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	calendars, err := graph.ListCalendars(ctx, loginResult.AccessToken)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(calendars, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化日历列表失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历数量: %d\n", len(calendars))
	for _, calendar := range calendars {
		fmt.Fprintf(os.Stdout, "- %s | 默认=%v | 可编辑=%v | id=%s\n", calendar.Name, calendar.IsDefaultCalendar, calendar.CanEdit, calendar.ID)
	}
	return nil
}

// runContactsFolderRename 重命名联系人文件夹。
func runContactsFolderRename(args []string) error {
	fs := flag.NewFlagSet("contacts folder-rename", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "文件夹 ID")
	nameFlag := fs.String("name", "", "新文件夹名称")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*nameFlag) == "" {
		return errors.New("用法: outlookctl contacts folder-rename --id <文件夹ID> --name <新名称>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.RenameContactFolder(ctx, loginResult.AccessToken, *idFlag, *nameFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人文件夹已重命名为: %s\n", *nameFlag)
	return nil
}

// runMailDraftUpdate 更新草稿邮件。
func runMailDraftUpdate(args []string) error {
	fs := flag.NewFlagSet("mail draft-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "草稿 ID")
	toFlag := fs.String("to", "", "收件人，多个地址用逗号分隔")
	ccFlag := fs.String("cc", "", "抄送，多个地址用逗号分隔")
	bccFlag := fs.String("bcc", "", "密送，多个地址用逗号分隔")
	subjectFlag := fs.String("subject", "", "邮件主题")
	bodyFlag := fs.String("body", "", "邮件正文")
	attachFlag := fs.String("attach", "", "附件路径，多个用逗号分隔")
	htmlFlag := fs.Bool("html", false, "正文使用 HTML 格式")
	importanceFlag := fs.String("importance", "", "重要性: low/normal/high")
	readReceiptFlag := fs.Bool("read-receipt", false, "请求已读回执")
	deliveryReceiptFlag := fs.Bool("delivery-receipt", false, "请求送达回执")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl mail draft-update --id <草稿ID> --to <邮箱> --subject <主题> --body <正文>")
	}
	to := splitAddresses(*toFlag)
	if len(to) == 0 {
		return errors.New("更新草稿时至少需要一个收件人")
	}
	attachments, err := graph.LoadMailAttachments(splitAddresses(*attachFlag))
	if err != nil {
		return err
	}
	contentType := "Text"
	if *htmlFlag {
		contentType = "HTML"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	detail, err := graph.UpdateDraft(ctx, loginResult.AccessToken, *idFlag, graph.SendMailInput{
		To:                     to,
		Cc:                     splitAddresses(*ccFlag),
		Bcc:                    splitAddresses(*bccFlag),
		Subject:                *subjectFlag,
		Body:                   *bodyFlag,
		ContentType:            contentType,
		Attachments:            attachments,
		Importance:             *importanceFlag,
		RequestReadReceipt:     *readReceiptFlag,
		RequestDeliveryReceipt: *deliveryReceiptFlag,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "草稿已更新: %s\n", detail.Subject)
	fmt.Fprintf(os.Stdout, "草稿 ID: %s\n", detail.ID)
	return nil
}

// parseBirthday 把 YYYY-MM-DD 格式的生日转换为 Graph 日期时间结构。
func parseBirthday(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.UTC)
	if err != nil {
		return ""
	}
	return parsed.Format(time.RFC3339)
}

// parseAddress 把 street|city|state|postal|country 格式解析为联系人地址。
func parseAddress(value string) *graph.PhysicalAddress {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "|")
	for len(parts) < 5 {
		parts = append(parts, "")
	}
	return &graph.PhysicalAddress{
		Street:          strings.TrimSpace(parts[0]),
		City:            strings.TrimSpace(parts[1]),
		State:           strings.TrimSpace(parts[2]),
		PostalCode:      strings.TrimSpace(parts[3]),
		CountryOrRegion: strings.TrimSpace(parts[4]),
	}
}

// runContactsPermanentDelete 永久删除联系人。
func runContactsPermanentDelete(args []string) error {
	fs := flag.NewFlagSet("contacts permanent-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "联系人 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl contacts permanent-delete --id <联系人ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.PermanentDeleteContact(ctx, loginResult.AccessToken, *idFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人已永久删除: %s\n", *idFlag)
	return nil
}

// runContactsPhotoGet 读取联系人照片并保存到文件。
func runContactsPhotoGet(args []string) error {
	fs := flag.NewFlagSet("contacts photo-get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "联系人 ID")
	outFlag := fs.String("out", "", "保存照片的本地路径")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*outFlag) == "" {
		return errors.New("用法: outlookctl contacts photo-get --id <联系人ID> --out <文件路径>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	data, contentType, err := graph.GetContactPhoto(ctx, loginResult.AccessToken, *idFlag)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*outFlag, data, 0o600); err != nil {
		return fmt.Errorf("保存联系人照片失败: %w", err)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人照片已保存: %s\n", *outFlag)
	fmt.Fprintf(os.Stdout, "内容类型: %s\n", contentType)
	fmt.Fprintf(os.Stdout, "大小: %d 字节\n", len(data))
	return nil
}

// runContactsPhotoSet 上传联系人照片。
func runContactsPhotoSet(args []string) error {
	fs := flag.NewFlagSet("contacts photo-set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "联系人 ID")
	fileFlag := fs.String("file", "", "本地照片文件路径")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*fileFlag) == "" {
		return errors.New("用法: outlookctl contacts photo-set --id <联系人ID> --file <照片路径>")
	}
	data, err := os.ReadFile(*fileFlag)
	if err != nil {
		return fmt.Errorf("读取联系人照片失败: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(*fileFlag))
	if contentType == "" {
		contentType = "image/jpeg"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.SetContactPhoto(ctx, loginResult.AccessToken, *idFlag, data, contentType); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人照片已更新: %s\n", *fileFlag)
	fmt.Fprintf(os.Stdout, "内容类型: %s\n", contentType)
	return nil
}

// runCalendarAttachments 列出日历事件附件。
func runCalendarAttachments(args []string) error {
	fs := flag.NewFlagSet("calendar attachments", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl calendar attachments --id <事件ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	attachments, err := graph.ListCalendarEventAttachments(ctx, loginResult.AccessToken, *idFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(attachments, "", "  ")
		if err != nil {
			return fmt.Errorf("序列化日历附件列表失败: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "附件数量: %d\n", len(attachments))
	for _, attachment := range attachments {
		fmt.Fprintf(os.Stdout, "- %s | %s | %d 字节 | id=%s\n", attachment.Name, attachment.ContentType, attachment.Size, attachment.ID)
	}
	return nil
}

// runCalendarAttachmentAdd 给日历事件添加附件。
func runCalendarAttachmentAdd(args []string) error {
	fs := flag.NewFlagSet("calendar attachment-add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	fileFlag := fs.String("file", "", "本地附件文件路径")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*fileFlag) == "" {
		return errors.New("用法: outlookctl calendar attachment-add --id <事件ID> --file <附件路径>")
	}
	attachments, err := graph.LoadMailAttachments([]string{*fileFlag})
	if err != nil {
		return err
	}
	if len(attachments) == 0 {
		return fmt.Errorf("附件文件不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	attachment, err := graph.AddCalendarEventAttachment(ctx, loginResult.AccessToken, *idFlag, attachments[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历附件已添加: %s\n", attachment.Name)
	fmt.Fprintf(os.Stdout, "附件 ID: %s\n", attachment.ID)
	return nil
}

// runCalendarAttachmentGet 下载日历事件附件。
func runCalendarAttachmentGet(args []string) error {
	fs := flag.NewFlagSet("calendar attachment-get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	attachmentIDFlag := fs.String("attachment-id", "", "附件 ID")
	outFlag := fs.String("out", "", "保存附件的本地路径")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*attachmentIDFlag) == "" || strings.TrimSpace(*outFlag) == "" {
		return errors.New("用法: outlookctl calendar attachment-get --id <事件ID> --attachment-id <附件ID> --out <文件路径>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	data, contentType, err := graph.GetCalendarEventAttachment(ctx, loginResult.AccessToken, *idFlag, *attachmentIDFlag)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*outFlag, data, 0o600); err != nil {
		return fmt.Errorf("保存日历附件失败: %w", err)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历附件已保存: %s\n", *outFlag)
	fmt.Fprintf(os.Stdout, "内容类型: %s\n", contentType)
	fmt.Fprintf(os.Stdout, "大小: %d 字节\n", len(data))
	return nil
}

// runCalendarCancel 取消日历事件。
func runCalendarCancel(args []string) error {
	fs := flag.NewFlagSet("calendar cancel", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	commentFlag := fs.String("comment", "", "取消说明")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" {
		return errors.New("用法: outlookctl calendar cancel --id <事件ID> [--comment <说明>]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.CancelCalendarEvent(ctx, loginResult.AccessToken, *idFlag, *commentFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历事件已取消: %s\n", *idFlag)
	return nil
}

// runCalendarRespond 响应日历邀请。
func runCalendarRespond(args []string) error {
	fs := flag.NewFlagSet("calendar respond", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	responseFlag := fs.String("response", "", "响应类型: accept/decline/tentative")
	commentFlag := fs.String("comment", "", "响应说明")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*responseFlag) == "" {
		return errors.New("用法: outlookctl calendar respond --id <事件ID> --response accept|decline|tentative")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.RespondCalendarEvent(ctx, loginResult.AccessToken, *idFlag, *responseFlag, *commentFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历响应已发送: %s\n", *responseFlag)
	return nil
}

// runCalendarForward 转发日历事件。
func runCalendarForward(args []string) error {
	fs := flag.NewFlagSet("calendar forward", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	toFlag := fs.String("to", "", "转发收件人，多个用逗号分隔")
	commentFlag := fs.String("comment", "", "转发说明")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || len(splitAddresses(*toFlag)) == 0 {
		return errors.New("用法: outlookctl calendar forward --id <事件ID> --to <邮箱> [--comment <说明>]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.ForwardCalendarEvent(ctx, loginResult.AccessToken, *idFlag, *commentFlag, splitAddresses(*toFlag)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历事件已转发: %s\n", *idFlag)
	return nil
}

// runMailDraftAttachmentDelete 删除草稿附件。
func runMailDraftAttachmentDelete(args []string) error {
	fs := flag.NewFlagSet("mail draft-attachment-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "草稿 ID")
	attachmentIDFlag := fs.String("attachment-id", "", "附件 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*attachmentIDFlag) == "" {
		return errors.New("用法: outlookctl mail draft-attachment-delete --id <草稿ID> --attachment-id <附件ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteDraftAttachment(ctx, loginResult.AccessToken, *idFlag, *attachmentIDFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "草稿附件已删除: %s\n", *attachmentIDFlag)
	return nil
}

// runMailDraftAttachmentReplace 替换草稿附件。
func runMailDraftAttachmentReplace(args []string) error {
	fs := flag.NewFlagSet("mail draft-attachment-replace", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "草稿 ID")
	attachmentIDFlag := fs.String("attachment-id", "", "旧附件 ID")
	fileFlag := fs.String("file", "", "新附件文件路径")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*attachmentIDFlag) == "" || strings.TrimSpace(*fileFlag) == "" {
		return errors.New("用法: outlookctl mail draft-attachment-replace --id <草稿ID> --attachment-id <附件ID> --file <新附件>")
	}
	attachments, err := graph.LoadMailAttachments([]string{*fileFlag})
	if err != nil {
		return err
	}
	if len(attachments) == 0 {
		return errors.New("新附件文件不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	attachment, err := graph.ReplaceDraftAttachment(ctx, loginResult.AccessToken, *idFlag, *attachmentIDFlag, attachments[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "草稿附件已替换: %s\n", attachment.Name)
	fmt.Fprintf(os.Stdout, "新附件 ID: %s\n", attachment.ID)
	return nil
}

// runContactsExport 导出联系人到 CSV。
func runContactsExport(args []string) error {
	fs := flag.NewFlagSet("contacts export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outFlag := fs.String("out", "", "导出的文件路径")
	formatFlag := fs.String("format", "csv", "导出格式: csv 或 vcf")
	folderFlag := fs.String("folder", "", "联系人文件夹名称或 ID；不填时使用默认文件夹")
	maxFlag := fs.Int("max", 500, "最多导出多少个联系人，范围 1-1000")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*outFlag) == "" {
		return errors.New("用法: outlookctl contacts export --out <CSV文件> [--folder <文件夹>] [--max 500]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folderID := ""
	if strings.TrimSpace(*folderFlag) != "" {
		folderID, err = graph.ResolveContactFolderID(ctx, loginResult.AccessToken, *folderFlag)
		if err != nil {
			return err
		}
	}
	contacts, err := graph.ListContactsInFolderAll(ctx, loginResult.AccessToken, folderID, *maxFlag)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(*formatFlag)) {
	case "csv", "":
		if err := contactcsv.ExportFile(*outFlag, contacts); err != nil {
			return err
		}
	case "vcf", "vcard":
		if err := vcard.ExportFile(*outFlag, contacts); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持的导出格式: %s", *formatFlag)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "联系人已导出: %d\n", len(contacts))
	fmt.Fprintf(os.Stdout, "文件: %s\n", *outFlag)
	return nil
}

// runContactsImport 从 CSV 导入联系人。
func runContactsImport(args []string) error {
	fs := flag.NewFlagSet("contacts import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fileFlag := fs.String("file", "", "联系人文件路径")
	formatFlag := fs.String("format", "csv", "导入格式: csv 或 vcf")
	folderFlag := fs.String("folder", "", "目标联系人文件夹名称或 ID；不填时使用默认文件夹")
	maxFlag := fs.Int("max", 500, "最多导入多少个联系人，范围 1-1000")
	confirmFlag := fs.Bool("confirm", false, "确认实际创建联系人；不填时只 dry-run")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*fileFlag) == "" {
		return errors.New("用法: outlookctl contacts import --file <CSV文件> [--folder <文件夹>] [--max 500] [--confirm]")
	}
	var inputs []graph.ContactInput
	var err error
	switch strings.ToLower(strings.TrimSpace(*formatFlag)) {
	case "csv", "":
		inputs, err = contactcsv.ReadFile(*fileFlag)
	case "vcf", "vcard":
		inputs, err = vcard.ReadFile(*fileFlag)
	default:
		return fmt.Errorf("不支持的导入格式: %s", *formatFlag)
	}
	if err != nil {
		return err
	}
	if *maxFlag <= 0 {
		*maxFlag = 500
	}
	if *maxFlag > 1000 {
		*maxFlag = 1000
	}
	if len(inputs) > *maxFlag {
		inputs = inputs[:*maxFlag]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folderID := ""
	if strings.TrimSpace(*folderFlag) != "" {
		folderID, err = graph.ResolveContactFolderID(ctx, loginResult.AccessToken, *folderFlag)
		if err != nil {
			return err
		}
	}
	if !*confirmFlag {
		fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
		fmt.Fprintf(os.Stdout, "待导入联系人: %d\n", len(inputs))
		fmt.Fprintln(os.Stdout, "这是 dry-run，未创建任何联系人；确认后加 --confirm 执行")
		return nil
	}
	successCount := 0
	failureCount := 0
	for _, input := range inputs {
		if _, err := graph.CreateContactInFolder(ctx, loginResult.AccessToken, folderID, input); err != nil {
			failureCount++
			fmt.Fprintf(os.Stderr, "导入失败 (%s): %v\n", input.DisplayName, err)
			continue
		}
		successCount++
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "导入成功: %d\n", successCount)
	fmt.Fprintf(os.Stdout, "导入失败: %d\n", failureCount)
	return nil
}

// runCalendarAttachmentDelete 删除日历附件。
func runCalendarAttachmentDelete(args []string) error {
	fs := flag.NewFlagSet("calendar attachment-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	attachmentIDFlag := fs.String("attachment-id", "", "附件 ID")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*attachmentIDFlag) == "" {
		return errors.New("用法: outlookctl calendar attachment-delete --id <事件ID> --attachment-id <附件ID>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	if err := graph.DeleteCalendarEventAttachment(ctx, loginResult.AccessToken, *idFlag, *attachmentIDFlag); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历附件已删除: %s\n", *attachmentIDFlag)
	return nil
}

// runCalendarAttachmentReplace 替换日历附件。
func runCalendarAttachmentReplace(args []string) error {
	fs := flag.NewFlagSet("calendar attachment-replace", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	attachmentIDFlag := fs.String("attachment-id", "", "旧附件 ID")
	fileFlag := fs.String("file", "", "新附件文件路径")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*idFlag) == "" || strings.TrimSpace(*attachmentIDFlag) == "" || strings.TrimSpace(*fileFlag) == "" {
		return errors.New("用法: outlookctl calendar attachment-replace --id <事件ID> --attachment-id <附件ID> --file <新附件>")
	}
	attachments, err := graph.LoadMailAttachments([]string{*fileFlag})
	if err != nil {
		return err
	}
	if len(attachments) == 0 {
		return errors.New("新附件文件不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	attachment, err := graph.ReplaceCalendarEventAttachment(ctx, loginResult.AccessToken, *idFlag, *attachmentIDFlag, attachments[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "日历附件已替换: %s\n", attachment.Name)
	fmt.Fprintf(os.Stdout, "新附件 ID: %s\n", attachment.ID)
	return nil
}

func printBulkResult(username string, result *graph.BulkResult, jsonFlag bool) error {
	if jsonFlag {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", username)
	fmt.Fprintf(os.Stdout, "总数: %d\n", result.Total)
	fmt.Fprintf(os.Stdout, "成功: %d\n", result.Succeeded)
	fmt.Fprintf(os.Stdout, "失败: %d\n", result.Failed)
	if result.DryRun {
		fmt.Fprintln(os.Stdout, "模式: dry-run，未修改任何数据；确认后加 --confirm 执行")
	}
	for _, failure := range result.Failures {
		fmt.Fprintf(os.Stderr, "失败: %s\n", failure)
	}
	return nil
}

func runMailBulkMove(args []string) error {
	fs := flag.NewFlagSet("mail bulk-move", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idsFlag := fs.String("ids", "", "邮件 ID 列表，多个用逗号分隔")
	folderFlag := fs.String("folder", "", "目标文件夹名称或 ID")
	confirmFlag := fs.Bool("confirm", false, "确认实际移动邮件")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(*idsFlag)
	if len(ids) == 0 || strings.TrimSpace(*folderFlag) == "" {
		return errors.New("用法: outlookctl mail bulk-move --ids <id1,id2> --folder <文件夹> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folderID, err := graph.ResolveMailFolderID(ctx, loginResult.AccessToken, *folderFlag)
	if err != nil {
		return err
	}
	result, err := graph.BulkMoveMessages(ctx, loginResult.AccessToken, ids, folderID, *confirmFlag)
	if err != nil {
		return err
	}
	return printBulkResult(selected.Username, result, *jsonFlag)
}

func runMailBulkDelete(args []string) error {
	fs := flag.NewFlagSet("mail bulk-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idsFlag := fs.String("ids", "", "邮件 ID 列表，多个用逗号分隔")
	confirmFlag := fs.Bool("confirm", false, "确认实际删除邮件")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(*idsFlag)
	if len(ids) == 0 {
		return errors.New("用法: outlookctl mail bulk-delete --ids <id1,id2> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	result, err := graph.BulkDeleteMessages(ctx, loginResult.AccessToken, ids, *confirmFlag)
	if err != nil {
		return err
	}
	return printBulkResult(selected.Username, result, *jsonFlag)
}

func runContactsBulkDelete(args []string) error {
	fs := flag.NewFlagSet("contacts bulk-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idsFlag := fs.String("ids", "", "联系人 ID 列表，多个用逗号分隔")
	confirmFlag := fs.Bool("confirm", false, "确认实际删除联系人")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(*idsFlag)
	if len(ids) == 0 {
		return errors.New("用法: outlookctl contacts bulk-delete --ids <id1,id2> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	result, err := graph.BulkDeleteContacts(ctx, loginResult.AccessToken, ids, *confirmFlag)
	if err != nil {
		return err
	}
	return printBulkResult(selected.Username, result, *jsonFlag)
}

func runCalendarAttachmentBulkDelete(args []string) error {
	fs := flag.NewFlagSet("calendar attachment-bulk-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "事件 ID")
	attachmentsFlag := fs.String("attachment-ids", "", "附件 ID 列表，多个用逗号分隔")
	confirmFlag := fs.Bool("confirm", false, "确认实际删除附件")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(*attachmentsFlag)
	if strings.TrimSpace(*idFlag) == "" || len(ids) == 0 {
		return errors.New("用法: outlookctl calendar attachment-bulk-delete --id <事件ID> --attachment-ids <id1,id2> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	result, err := graph.BulkDeleteCalendarEventAttachments(ctx, loginResult.AccessToken, *idFlag, ids, *confirmFlag)
	if err != nil {
		return err
	}
	return printBulkResult(selected.Username, result, *jsonFlag)
}

func runMailDraftAttachmentBulkDelete(args []string) error {
	fs := flag.NewFlagSet("mail draft-attachment-bulk-delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	idFlag := fs.String("id", "", "草稿 ID")
	attachmentsFlag := fs.String("attachment-ids", "", "附件 ID 列表，多个用逗号分隔")
	confirmFlag := fs.Bool("confirm", false, "确认实际删除附件")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ids := splitAddresses(*attachmentsFlag)
	if strings.TrimSpace(*idFlag) == "" || len(ids) == 0 {
		return errors.New("用法: outlookctl mail draft-attachment-bulk-delete --id <草稿ID> --attachment-ids <id1,id2> [--confirm]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	result, err := graph.BulkDeleteDraftAttachments(ctx, loginResult.AccessToken, *idFlag, ids, *confirmFlag)
	if err != nil {
		return err
	}
	return printBulkResult(selected.Username, result, *jsonFlag)
}

func runMailFolderTree(args []string) error {
	fs := flag.NewFlagSet("mail folder-tree", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	topFlag := fs.Int("top", 200, "每层最多读取多少个文件夹")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	if err != nil {
		return err
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return err
	}
	selected, err := auth.SelectAccount(accounts, *accountFlag)
	if err != nil {
		return err
	}
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	tree, err := graph.ListMailFolderTree(ctx, loginResult.AccessToken, *topFlag)
	if err != nil {
		return err
	}
	if *jsonFlag {
		data, err := json.MarshalIndent(tree, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	var printNode func(nodes []graph.MailFolderNode, depth int)
	printNode = func(nodes []graph.MailFolderNode, depth int) {
		for _, node := range nodes {
			fmt.Fprintf(os.Stdout, "%s- %s | 未读=%d | 总数=%d | id=%s\n", strings.Repeat("  ", depth), node.DisplayName, node.UnreadItemCount, node.TotalItemCount, node.ID)
			printNode(node.Children, depth+1)
		}
	}
	printNode(tree, 0)
	return nil
}

// printUsage 输出命令行帮助。
func printUsage() {
	fmt.Fprintln(os.Stdout, `outlookctl - Outlook Manager 命令行工具

用法:
  outlookctl auth login                    通过 Device Code 登录并保存账号
  outlookctl auth status                   验证本地 token 缓存和静默刷新
  outlookctl account list                  查看本地账号档案
  outlookctl account use <email>           切换默认账号
  outlookctl mailbox settings             查看邮箱设置
  outlookctl mailbox auto-reply get       查看自动回复
  outlookctl mailbox auto-reply set       设置自动回复
  outlookctl mailbox timezone get         查看时区
  outlookctl mailbox timezone set         设置时区
  outlookctl doctor                        执行环境与账号诊断
  outlookctl update check                  检查 GitHub 最新版本
  outlookctl update download               下载并校验当前平台更新
  outlookctl update apply --yes            备份并替换当前二进制
  outlookctl update rollback --yes         使用备份回滚
  outlookctl config get|set                查看或设置 CLI 偏好
  outlookctl completion <shell>            生成 Shell 自动补全
  outlookctl ui                            打开交互式文本菜单
  outlookctl sync run                     同步本地索引
  outlookctl sync status                  查看本地索引状态
  outlookctl sync reset --account <邮箱> --confirm  删除单账号索引
  outlookctl sync reset --all --confirm   删除全部索引
  outlookctl sync compact                 压缩和优化 SQLite 索引
  outlookctl search --query <关键字>       搜索本地索引
  outlookctl audit list [--limit 50]       查询本地审计日志
  outlookctl audit export --out <文件> --format csv|json  导出审计日志
  outlookctl audit stats                   审计统计
  outlookctl audit verify                  校验审计哈希链
  outlookctl audit config get|set          查看或设置保留策略
  outlookctl audit cleanup                 清理过期审计日志
  outlookctl mail list [--account <email>] 读取收件箱最新邮件
  outlookctl mail search --query <关键字>       搜索邮件
  outlookctl mail query --from <邮箱> --unread --has-attachment --after 2026-10-01  高级邮件查询
  outlookctl mail view save <名称> --from <邮箱> --unread  保存邮件视图
  outlookctl mail view run <名称>                    运行邮件视图
  outlookctl mail send --to <邮箱> ...          发送邮件
  outlookctl mail read <message-id>         读取单封邮件详情
  outlookctl mail mark-read <message-id>    标记邮件为已读
  outlookctl mail mark-unread <message-id>  标记邮件为未读
  outlookctl mail folders                    列出邮件文件夹
  outlookctl mail folder-tree                以树形列出邮件文件夹
  outlookctl mail bulk-move --ids <id1,id2> --folder <文件夹> [--confirm]  批量移动邮件
  outlookctl mail bulk-delete --ids <id1,id2> [--confirm]  批量删除邮件
  outlookctl mail bulk-mark-read --ids <id1,id2> [--confirm]  批量标记已读
  outlookctl mail bulk-mark-unread --ids <id1,id2> [--confirm]  批量标记未读
  outlookctl mail bulk-flag --ids <id1,id2> [--confirm]  批量设置旗标
  outlookctl mail bulk-unflag --ids <id1,id2> [--confirm]  批量取消旗标
  outlookctl mail bulk-importance --ids <id1,id2> --level high [--confirm]  批量设置重要性
  outlookctl mail bulk-category-add --ids <id1,id2> --category <名称> [--confirm]  批量添加类别
  outlookctl mail bulk-category-remove --ids <id1,id2> --category <名称> [--confirm]  批量移除类别
  outlookctl mail draft-attachment-bulk-delete --id <草稿ID> --attachment-ids <id1,id2> [--confirm]  批量删除草稿附件
  outlookctl mail move <message-id> --folder <文件夹>  移动邮件
  outlookctl mail delete <message-id>        删除邮件
  outlookctl mail permanent-delete --id <邮件ID>  永久删除邮件
  outlookctl mail attachments <message-id>   列出邮件附件
  outlookctl mail attachment-save <message-id> --attachment <id> --out <path>  保存附件
  outlookctl mail export-eml <邮件ID> --out <文件>  导出 EML/MIME
  outlookctl mail import-eml --file <EML文件> [--confirm]  导入 EML 为草稿
  outlookctl calendar list --days 7          查看未来日历事件
  outlookctl calendar calendars              列出日历
  outlookctl calendar attachments --id <事件ID>  列出事件附件
  outlookctl calendar attachment-add --id <事件ID> --file <文件>  添加事件附件
  outlookctl calendar attachment-get --id <事件ID> --attachment-id <附件ID> --out <文件>  下载事件附件
  outlookctl calendar attachment-delete --id <事件ID> --attachment-id <附件ID>  删除事件附件
  outlookctl calendar attachment-replace --id <事件ID> --attachment-id <附件ID> --file <文件>  替换事件附件
  outlookctl calendar attachment-bulk-delete --id <事件ID> --attachment-ids <id1,id2> [--confirm]  批量删除事件附件
  outlookctl calendar list --calendar <名称或ID>  查看指定日历事件
  outlookctl calendar export-ics --out <ICS文件>  导出日历事件
  outlookctl calendar import-ics --file <ICS文件> [--confirm]  导入日历事件
  outlookctl calendar series-info --id <事件ID>  查看重复事件信息
  outlookctl calendar series-update --id <事件ID> --scope single|series  更新单次或整个系列
  outlookctl calendar series-delete --id <事件ID> --scope single|series  删除单次或整个系列
  outlookctl calendar freebusy --schedules <邮箱列表> --start <时间> --end <时间>  查询空闲状态
  outlookctl calendar find-time --attendees <邮箱列表> --start <时间> --end <时间>  查找会议时间
  outlookctl calendar create --subject <主题> --start <时间> --end <时间>  创建事件
  outlookctl calendar update --id <事件ID> --subject <主题> --start <时间> --end <时间>  更新事件
  outlookctl calendar delete --id <事件ID>  删除事件
  outlookctl calendar cancel --id <事件ID>  取消事件
  outlookctl calendar respond --id <事件ID> --response accept  响应日历邀请
  outlookctl calendar forward --id <事件ID> --to <邮箱>  转发日历事件
  outlookctl contacts list --top 50          查看联系人
  outlookctl contacts search --query <关键字>  搜索联系人
  outlookctl contacts folders               列出联系人文件夹
  outlookctl contacts folder-create --name <名称>  创建联系人文件夹
  outlookctl contacts folder-rename --id <文件夹ID> --name <新名称>  重命名联系人文件夹
  outlookctl contacts folder-delete --id <文件夹ID>  删除联系人文件夹
  outlookctl contacts create --display-name <名称>  创建联系人
  outlookctl contacts update --id <联系人ID> --display-name <名称>  更新联系人
  outlookctl contacts delete --id <联系人ID>  删除联系人
  outlookctl contacts permanent-delete --id <联系人ID>  永久删除联系人
  outlookctl contacts photo-get --id <联系人ID> --out <文件>  下载联系人照片
  outlookctl contacts photo-set --id <联系人ID> --file <文件>  上传联系人照片
  outlookctl contacts move --id <联系人ID> --folder <文件夹>  移动联系人
  outlookctl contacts copy --id <联系人ID> --folder <文件夹>  复制联系人
  outlookctl contacts export --out <CSV文件>  导出联系人
  outlookctl contacts import --file <CSV文件> [--confirm]  导入联系人
  outlookctl contacts bulk-delete --ids <id1,id2> [--confirm]  批量删除联系人
  outlookctl contacts duplicates scan --strategy email  扫描重复联系人
  outlookctl contacts duplicates merge --strategy email [--confirm]  合并重复联系人
  outlookctl mail reply <message-id> --body <正文>           回复邮件
  outlookctl mail reply-all <message-id> --body <正文>       回复全部
  outlookctl mail forward <message-id> --to <邮箱>          转发邮件
  outlookctl mail draft-create --to <邮箱> --subject <主题> --body <正文>  创建草稿
  outlookctl mail drafts                     列出草稿
  outlookctl mail draft-send <draft-id>      发送草稿
  outlookctl mail draft-update --id <草稿ID>  更新草稿
  outlookctl mail draft-attachment-delete --id <草稿ID> --attachment-id <附件ID>  删除草稿附件
  outlookctl mail draft-attachment-replace --id <草稿ID> --attachment-id <附件ID> --file <文件>  替换草稿附件
  outlookctl mail folder-create --name <名称>  创建文件夹
  outlookctl mail folder-rename <id> --name <名称>  重命名文件夹
  outlookctl mail folder-delete <id>         删除文件夹
  outlookctl mail flag <message-id>         设置邮件旗标
  outlookctl mail unflag <message-id>       取消邮件旗标
  outlookctl mail importance <message-id> --level low|normal|high  设置重要性
  outlookctl mail message-categories <id>    列出邮件类别
  outlookctl mail category-add <id> --category <名称>  添加类别
  outlookctl mail category-remove <id> --category <名称>  移除类别
  outlookctl mail rules                       列出收件箱规则
  outlookctl mail rule-create --name <名称>   创建收件箱规则
  outlookctl mail rule-create --name <名称> --apply-now --confirm-apply  创建并立即回放规则
  outlookctl mail rule-update --id <规则ID> --name <名称>  更新收件箱规则
  outlookctl mail rule-apply --id <规则ID> [--confirm]  回放规则到已有邮件
  outlookctl mail rule-delete --id <规则ID>   删除收件箱规则
  outlookctl mail master-categories           列出主类别
  outlookctl mail master-category-create --name <名称>  创建主类别
  outlookctl mail master-category-update --id <类别ID> --color preset5  更新主类别颜色
  outlookctl mail master-category-delete --id <类别ID>  删除主类别
  outlookctl version                       查看版本`)
}
