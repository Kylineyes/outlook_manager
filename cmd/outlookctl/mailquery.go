package main

import (
	"context"
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

type mailQueryOptions struct {
	folder        string
	from          string
	to            string
	subject       string
	body          string
	search        string
	importance    string
	category      string
	after         string
	before        string
	unreadOnly    bool
	hasAttachment bool
	top           int
	account       string
	json          bool
}

func addMailQueryFlags(fs *flag.FlagSet, options *mailQueryOptions) {
	fs.StringVar(&options.folder, "folder", "inbox", "文件夹名称或 ID；留空表示全邮箱")
	fs.StringVar(&options.from, "from", "", "发件人邮箱或关键字")
	fs.StringVar(&options.to, "to", "", "收件人邮箱或姓名关键字")
	fs.StringVar(&options.subject, "subject", "", "主题关键字")
	fs.StringVar(&options.body, "body", "", "正文预览关键字")
	fs.StringVar(&options.search, "search", "", "主题加正文关键字")
	fs.StringVar(&options.importance, "importance", "", "重要性: low/normal/high")
	fs.StringVar(&options.category, "category", "", "类别名称")
	fs.StringVar(&options.after, "after", "", "开始时间，YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss")
	fs.StringVar(&options.before, "before", "", "结束时间，YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss")
	fs.BoolVar(&options.unreadOnly, "unread", false, "仅未读邮件")
	fs.BoolVar(&options.hasAttachment, "has-attachment", false, "仅带附件邮件")
	fs.IntVar(&options.top, "top", 100, "最多返回多少封邮件，范围 1-1000")
	fs.StringVar(&options.account, "account", "", "指定账号邮箱或 HomeAccountID")
	fs.BoolVar(&options.json, "json", false, "以 JSON 格式输出")
}

func normalizeQueryTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	layouts := []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("无法解析时间: %s", value)
}

func (options mailQueryOptions) toGraphQuery() (graph.MessageQuery, error) {
	after, err := normalizeQueryTime(options.after)
	if err != nil {
		return graph.MessageQuery{}, err
	}
	before, err := normalizeQueryTime(options.before)
	if err != nil {
		return graph.MessageQuery{}, err
	}
	folder := strings.TrimSpace(options.folder)
	if strings.EqualFold(folder, "all") {
		folder = ""
	}
	return graph.MessageQuery{
		Folder:        folder,
		From:          options.from,
		To:            options.to,
		Subject:       options.subject,
		Body:          options.body,
		Search:        options.search,
		Importance:    options.importance,
		Category:      options.category,
		After:         after,
		Before:        before,
		UnreadOnly:    options.unreadOnly,
		HasAttachment: options.hasAttachment,
		Top:           options.top,
	}, nil
}

func runMailQuery(args []string) error {
	fs := flag.NewFlagSet("mail query", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options mailQueryOptions
	addMailQueryFlags(fs, &options)
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	query, err := options.toGraphQuery()
	if err != nil {
		return err
	}
	return executeMailQuery(query, options.account, options.json)
}

func executeMailQuery(query graph.MessageQuery, account string, jsonOutput bool) error {
	if query.Top <= 0 {
		query.Top = 100
	}
	if query.Top > 1000 {
		query.Top = 1000
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
	login, err := client.AcquireTokenSilent(ctx, selected)
	if err != nil {
		return err
	}
	folderID := strings.TrimSpace(query.Folder)
	if folderID != "" {
		folderID, err = graph.ResolveMailFolderID(ctx, login.AccessToken, folderID)
		if err != nil {
			return err
		}
		query.Folder = folderID
	}
	messages, err := graph.QueryMessages(ctx, login.AccessToken, query)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(messages)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	if strings.TrimSpace(query.Folder) != "" {
		fmt.Fprintf(os.Stdout, "文件夹: %s\n", query.Folder)
	}
	fmt.Fprintf(os.Stdout, "邮件数量: %d\n", len(messages))
	for i, message := range messages {
		readState := "未读"
		if message.IsRead {
			readState = "已读"
		}
		attachmentState := ""
		if message.HasAttachments {
			attachmentState = " | 附件"
		}
		fmt.Fprintf(os.Stdout, "%d. [%s] %s | %s%s\n", i+1, message.ReceivedDateTime.Format("2006-01-02 15:04"), readState, message.Subject, attachmentState)
	}
	return nil
}

func runMailView(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl mail view <save|list|run|delete> [参数]")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl mail view <save|list|run|delete> [参数]")
	}
	switch args[0] {
	case "save":
		return runMailViewSave(args[1:])
	case "list":
		return runMailViewList(args[1:])
	case "run":
		return runMailViewRun(args[1:])
	case "delete":
		return runMailViewDelete(args[1:])
	default:
		return fmt.Errorf("未知 mail view 子命令: %s", args[0])
	}
}

func runMailViewSave(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: outlookctl mail view save <名称> [查询条件]")
	}
	name := strings.TrimSpace(args[0])
	if name == "" {
		return errors.New("视图名称不能为空")
	}
	fs := flag.NewFlagSet("mail view save", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var options mailQueryOptions
	addMailQueryFlags(fs, &options)
	if err := parseCommandFlags(fs, args[1:]); err != nil {
		return err
	}
	_, err := options.toGraphQuery()
	if err != nil {
		return err
	}
	view := store.MailView{
		Name:          name,
		Folder:        options.folder,
		From:          options.from,
		To:            options.to,
		Subject:       options.subject,
		Body:          options.body,
		Search:        options.search,
		Importance:    options.importance,
		Category:      options.category,
		After:         options.after,
		Before:        options.before,
		UnreadOnly:    options.unreadOnly,
		HasAttachment: options.hasAttachment,
		Top:           options.top,
	}
	if _, err := store.UpsertMailView(view); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "邮件视图已保存: %s\n", name)
	return nil
}

func runMailViewList(args []string) error {
	fs := flag.NewFlagSet("mail view list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	file, err := store.LoadMailViews()
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(file.Views)
	}
	fmt.Fprintf(os.Stdout, "视图数量: %d\n", len(file.Views))
	for i, view := range file.Views {
		conditions := make([]string, 0, 4)
		if view.From != "" {
			conditions = append(conditions, "from="+view.From)
		}
		if view.Subject != "" {
			conditions = append(conditions, "subject="+view.Subject)
		}
		if view.UnreadOnly {
			conditions = append(conditions, "unread")
		}
		if view.HasAttachment {
			conditions = append(conditions, "hasAttachment")
		}
		fmt.Fprintf(os.Stdout, "%d. %s | %s\n", i+1, view.Name, strings.Join(conditions, ","))
	}
	return nil
}

func runMailViewRun(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: outlookctl mail view run <名称>")
	}
	name := strings.TrimSpace(args[0])
	file, err := store.LoadMailViews()
	if err != nil {
		return err
	}
	for _, view := range file.Views {
		if strings.EqualFold(view.Name, name) {
			options := mailQueryOptions{
				folder: view.Folder, from: view.From, to: view.To, subject: view.Subject,
				body: view.Body, search: view.Search, importance: view.Importance,
				category: view.Category, after: view.After, before: view.Before,
				unreadOnly: view.UnreadOnly, hasAttachment: view.HasAttachment,
				top: view.Top,
			}
			query, err := options.toGraphQuery()
			if err != nil {
				return err
			}
			if query.Top == 0 {
				query.Top = 100
			}
			return executeMailQuery(query, "", false)
		}
	}
	return fmt.Errorf("找不到邮件视图: %s", name)
}

func runMailViewDelete(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: outlookctl mail view delete <名称>")
	}
	name := strings.TrimSpace(args[0])
	if _, err := store.DeleteMailView(name); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "邮件视图已删除: %s\n", name)
	return nil
}
