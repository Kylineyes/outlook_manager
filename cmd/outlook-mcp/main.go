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
