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
	case "read":
		return runMailRead(args[1:])
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
func runMailRead(args []string) error {
	fs := flag.NewFlagSet("mail read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("用法: outlookctl mail read <message-id>")
	}
	messageID := fs.Arg(0)

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
	detail, err := graph.GetMessage(ctx, loginResult.AccessToken, messageID)
	if err != nil {
		return err
	}

	if *jsonFlag {
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
  outlookctl mail read <message-id>         读取单封邮件详情
  outlookctl version                       查看版本`)
}
