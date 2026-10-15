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
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/auth"
	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

// version 表示当前构建版本。
// 默认值用于本地开发；正式发布时由 GitHub Actions 通过 -ldflags 注入 tag。
var version = "dev"

// main 负责捕获错误并设置进程退出码。
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "outlookctl: %v\n", err)
		os.Exit(1)
	}
}

// run 根据第一个参数分发子命令。
func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "auth":
		return runAuth(args[1:])
	case "account":
		return runAccount(args[1:])
	case "mail":
		return runMail(args[1:])
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

// runAuth 处理 auth 子命令。
func runAuth(args []string) error {
	if len(args) == 0 {
		printUsage()
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
	if len(args) == 0 {
		printUsage()
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
	if len(args) == 0 {
		printUsage()
		return errors.New("mail 需要指定子命令: list")
	}
	switch args[0] {
	case "list":
		return runMailList(args[1:])
	case "search":
		return runMailSearch(args[1:])
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
	case "move":
		return runMailMove(args[1:])
	case "delete":
		return runMailDelete(args[1:])
	case "attachments":
		return runMailAttachments(args[1:])
	case "attachment-save":
		return runMailAttachmentSave(args[1:])
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
	default:
		printUsage()
		return fmt.Errorf("未知 mail 子命令: %s", args[0])
	}
}

// runMailList 读取当前账号收件箱最新邮件。
func runMailList(args []string) error {
	fs := flag.NewFlagSet("mail list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	topFlag := fs.Int("top", 10, "最多返回多少封邮件，范围 1-100")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := fs.Parse(args); err != nil {
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

	// 先静默刷新一次，确保 access token 可用，并顺带验证 refresh token。
	loginResult, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	messages, err := graph.ListInboxMessages(ctx, loginResult.AccessToken, *topFlag)
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

	fmt.Fprintf(os.Stdout, "账号: %s\n", selected.Username)
	fmt.Fprintf(os.Stdout, "收件箱最新 %d 封邮件:\n", len(messages))
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

// runMailSearch 在全部邮箱文件夹中搜索邮件。
func runMailSearch(args []string) error {
	fs := flag.NewFlagSet("mail search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	queryFlag := fs.String("query", "", "搜索关键字")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	topFlag := fs.Int("top", 10, "最多返回多少封邮件，范围 1-100")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := fs.Parse(args); err != nil {
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
	messages, err := graph.SearchMessages(ctx, loginResult.AccessToken, *queryFlag, *topFlag)
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
	htmlFlag := fs.Bool("html", false, "正文使用 HTML 格式")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	noSaveFlag := fs.Bool("no-save", false, "不保存到已发送邮件")
	if err := fs.Parse(args); err != nil {
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
		To:              to,
		Cc:              splitAddresses(*ccFlag),
		Bcc:             splitAddresses(*bccFlag),
		Subject:         *subjectFlag,
		Body:            body,
		ContentType:     contentType,
		SaveToSentItems: !*noSaveFlag,
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
	if err := fs.Parse(args); err != nil {
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
	htmlFlag := fs.Bool("html", false, "正文使用 HTML 格式")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	to := splitAddresses(*toFlag)
	if len(to) == 0 {
		return errors.New("用法: outlookctl mail draft-create --to <邮箱> --subject <主题> --body <正文>")
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
		To:          to,
		Cc:          splitAddresses(*ccFlag),
		Bcc:         splitAddresses(*bccFlag),
		Subject:     *subjectFlag,
		Body:        *bodyFlag,
		ContentType: contentType,
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
	if err := fs.Parse(args); err != nil {
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
	if err := fs.Parse(args); err != nil {
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

// printUsage 输出命令行帮助。
func printUsage() {
	fmt.Fprintln(os.Stdout, `outlookctl - Outlook Manager 命令行工具

用法:
  outlookctl auth login                    通过 Device Code 登录并保存账号
  outlookctl auth status                   验证本地 token 缓存和静默刷新
  outlookctl account list                  查看本地账号档案
  outlookctl account use <email>           切换默认账号
  outlookctl mail list [--account <email>] 读取收件箱最新邮件
  outlookctl mail search --query <关键字>       搜索邮件
  outlookctl mail send --to <邮箱> ...          发送邮件
  outlookctl mail read <message-id>         读取单封邮件详情
  outlookctl mail mark-read <message-id>    标记邮件为已读
  outlookctl mail mark-unread <message-id>  标记邮件为未读
  outlookctl mail folders                    列出邮件文件夹
  outlookctl mail move <message-id> --folder <文件夹>  移动邮件
  outlookctl mail delete <message-id>        删除邮件
  outlookctl mail attachments <message-id>   列出邮件附件
  outlookctl mail attachment-save <message-id> --attachment <id> --out <path>  保存附件
  outlookctl mail reply <message-id> --body <正文>           回复邮件
  outlookctl mail reply-all <message-id> --body <正文>       回复全部
  outlookctl mail forward <message-id> --to <邮箱>          转发邮件
  outlookctl mail draft-create --to <邮箱> --subject <主题> --body <正文>  创建草稿
  outlookctl mail drafts                     列出草稿
  outlookctl mail draft-send <draft-id>      发送草稿
  outlookctl mail folder-create --name <名称>  创建文件夹
  outlookctl mail folder-rename <id> --name <名称>  重命名文件夹
  outlookctl mail folder-delete <id>         删除文件夹
  outlookctl mail flag <message-id>         设置邮件旗标
  outlookctl mail unflag <message-id>       取消邮件旗标
  outlookctl mail importance <message-id> --level low|normal|high  设置重要性
  outlookctl version                       查看版本`)
}
