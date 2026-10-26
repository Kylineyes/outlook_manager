package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/audit"
	"github.com/Kylineyes/outlook-manager/internal/eml"
	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func runMailExportEML(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: outlookctl mail export-eml <邮件ID> --out <文件>")
	}
	messageID := strings.TrimSpace(args[0])
	fs := flag.NewFlagSet("mail export-eml", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outFlag := fs.String("out", "", "导出的 EML 文件路径")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args[1:]); err != nil {
		return err
	}
	if messageID == "" || strings.TrimSpace(*outFlag) == "" {
		return errors.New("用法: outlookctl mail export-eml <邮件ID> --out <文件>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	data, err := graph.GetMessageMIME(ctx, login.AccessToken, messageID)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(*outFlag); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("创建导出目录失败: %w", err)
		}
	}
	if err := os.WriteFile(*outFlag, data, 0o600); err != nil {
		return fmt.Errorf("写入 EML 文件失败: %w", err)
	}
	audit.Log("mail.eml.export", map[string]any{"messageId": messageID, "size": len(data)})
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "EML 已导出: %s\n", *outFlag)
	fmt.Fprintf(os.Stdout, "大小: %d 字节\n", len(data))
	return nil
}

func runMailImportEML(args []string) error {
	fs := flag.NewFlagSet("mail import-eml", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fileFlag := fs.String("file", "", "要导入的 EML 文件路径")
	confirmFlag := fs.Bool("confirm", false, "确认创建草稿；不填时只 dry-run")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*fileFlag) == "" {
		return errors.New("用法: outlookctl mail import-eml --file <EML文件> [--confirm]")
	}
	parsed, err := eml.ParseFile(*fileFlag)
	if err != nil {
		return err
	}
	draft, err := eml.ToDraftInput(parsed)
	if err != nil {
		return err
	}
	defer draft.Cleanup()
	attachments := make([]map[string]any, 0, len(draft.Input.Attachments))
	for _, attachment := range draft.Input.Attachments {
		attachments = append(attachments, map[string]any{"name": attachment.Name, "size": attachment.Size})
	}
	if !*confirmFlag {
		if *jsonFlag {
			return printJSON(map[string]any{
				"success": true, "dryRun": true, "subject": draft.Input.Subject,
				"to": draft.Input.To, "cc": draft.Input.Cc, "bcc": draft.Input.Bcc,
				"bodyType": draft.Input.ContentType, "attachments": attachments,
			})
		}
		fmt.Fprintln(os.Stdout, "模式: dry-run，未创建草稿；确认后加 --confirm 执行")
		fmt.Fprintf(os.Stdout, "主题: %s\n", draft.Input.Subject)
		fmt.Fprintf(os.Stdout, "收件人: %s\n", strings.Join(draft.Input.To, ", "))
		fmt.Fprintf(os.Stdout, "抄送: %s\n", strings.Join(draft.Input.Cc, ", "))
		fmt.Fprintf(os.Stdout, "密送: %s\n", strings.Join(draft.Input.Bcc, ", "))
		fmt.Fprintf(os.Stdout, "正文类型: %s\n", draft.Input.ContentType)
		fmt.Fprintf(os.Stdout, "附件数量: %d\n", len(attachments))
		for _, attachment := range attachments {
			fmt.Fprintf(os.Stdout, "- %v (%v 字节)\n", attachment["name"], attachment["size"])
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	draftID, err := graph.CreateDraft(ctx, login.AccessToken, draft.Input)
	if err != nil {
		return err
	}
	audit.Log("mail.eml.import", map[string]any{"draftId": draftID, "attachmentCount": len(draft.Input.Attachments)})
	if *jsonFlag {
		return printJSON(map[string]any{"success": true, "dryRun": false, "account": login.Account.Username, "draftId": draftID, "attachments": attachments})
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "EML 已导入为草稿: %s\n", draftID)
	fmt.Fprintf(os.Stdout, "附件数量: %d\n", len(attachments))
	return nil
}
