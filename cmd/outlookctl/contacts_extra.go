package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/contactmerge"
	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func runContactsDuplicates(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: outlookctl contacts duplicates <scan|merge> [参数]")
	}
	switch args[0] {
	case "scan":
		return runContactsDuplicatesScan(args[1:])
	case "merge":
		return runContactsDuplicatesMerge(args[1:])
	default:
		return fmt.Errorf("未知 duplicates 子命令: %s", args[0])
	}
}

func loadContactsForMerge(ctx context.Context, accessToken, folder string, max int) ([]graph.Contact, error) {
	folderID := ""
	var err error
	if strings.TrimSpace(folder) != "" {
		folderID, err = graph.ResolveContactFolderID(ctx, accessToken, folder)
		if err != nil {
			return nil, err
		}
	}
	return graph.ListContactsInFolderAll(ctx, accessToken, folderID, max)
}

func runContactsDuplicatesScan(args []string) error {
	fs := flag.NewFlagSet("contacts duplicates scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	strategyFlag := fs.String("strategy", "email", "重复判断策略: email/phone/name")
	folderFlag := fs.String("folder", "", "联系人文件夹名称或 ID")
	maxFlag := fs.Int("max", 1000, "最多扫描多少个联系人")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	contacts, err := loadContactsForMerge(ctx, login.AccessToken, *folderFlag, *maxFlag)
	if err != nil {
		return err
	}
	groups := contactmerge.FindGroups(contacts, *strategyFlag)
	if *jsonFlag {
		return printJSON(groups)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "联系人数量: %d\n", len(contacts))
	fmt.Fprintf(os.Stdout, "重复组数量: %d\n", len(groups))
	for index, group := range groups {
		fmt.Fprintf(os.Stdout, "%d. [%s] %s\n", index+1, group.Strategy, group.Key)
		for _, contact := range group.Contacts {
			fmt.Fprintf(os.Stdout, "   - %s | %s | %s\n", contact.DisplayName, contact.ID, contactEmailSummary(contact))
		}
	}
	return nil
}

func runContactsDuplicatesMerge(args []string) error {
	fs := flag.NewFlagSet("contacts duplicates merge", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	strategyFlag := fs.String("strategy", "email", "重复判断策略: email/phone/name")
	folderFlag := fs.String("folder", "", "联系人文件夹名称或 ID")
	maxFlag := fs.Int("max", 1000, "最多扫描多少个联系人")
	confirmFlag := fs.Bool("confirm", false, "确认更新主联系人并删除重复联系人")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	accountFlag := fs.String("account", "", "指定账号邮箱或 HomeAccountID")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	_, login, err := mailboxLogin(ctx, *accountFlag)
	if err != nil {
		return err
	}
	contacts, err := loadContactsForMerge(ctx, login.AccessToken, *folderFlag, *maxFlag)
	if err != nil {
		return err
	}
	groups := contactmerge.FindGroups(contacts, *strategyFlag)
	type mergePlan struct {
		Key          string   `json:"key"`
		MasterID     string   `json:"masterId"`
		MasterName   string   `json:"masterName"`
		DuplicateIDs []string `json:"duplicateIds"`
	}
	plans := make([]mergePlan, 0, len(groups))
	for _, group := range groups {
		master, duplicateIDs := contactmerge.MergeGroup(group)
		plans = append(plans, mergePlan{Key: group.Key, MasterID: master.ID, MasterName: master.DisplayName, DuplicateIDs: duplicateIDs})
	}
	if !*confirmFlag {
		if *jsonFlag {
			return printJSON(map[string]any{"success": true, "dryRun": true, "account": login.Account.Username, "groups": plans})
		}
		fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
		fmt.Fprintf(os.Stdout, "重复组数量: %d\n", len(plans))
		fmt.Fprintln(os.Stdout, "模式: dry-run，未修改联系人；确认后加 --confirm 执行")
		for _, plan := range plans {
			fmt.Fprintf(os.Stdout, "- %s | 主联系人=%s | 待删除=%s\n", plan.Key, plan.MasterID, strings.Join(plan.DuplicateIDs, ","))
		}
		return nil
	}
	updated := 0
	deleted := 0
	failures := make([]string, 0)
	for _, group := range groups {
		master, duplicateIDs := contactmerge.MergeGroup(group)
		if _, err := graph.UpdateContact(ctx, login.AccessToken, master.ID, graph.ContactToInput(master)); err != nil {
			failures = append(failures, fmt.Sprintf("更新 %s 失败: %v", master.ID, err))
			continue
		}
		updated++
		for _, duplicateID := range duplicateIDs {
			if err := graph.DeleteContact(ctx, login.AccessToken, duplicateID); err != nil {
				failures = append(failures, fmt.Sprintf("删除 %s 失败: %v", duplicateID, err))
				continue
			}
			deleted++
		}
	}
	result := map[string]any{"success": len(failures) == 0, "dryRun": false, "account": login.Account.Username, "groups": len(groups), "updated": updated, "deleted": deleted, "failures": failures}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", login.Account.Username)
	fmt.Fprintf(os.Stdout, "更新主联系人: %d\n", updated)
	fmt.Fprintf(os.Stdout, "删除重复联系人: %d\n", deleted)
	fmt.Fprintf(os.Stdout, "失败: %d\n", len(failures))
	for _, failure := range failures {
		fmt.Fprintf(os.Stderr, "失败: %s\n", failure)
	}
	return nil
}

func contactEmailSummary(contact graph.Contact) string {
	emails := make([]string, 0, len(contact.EmailAddresses))
	for _, email := range contact.EmailAddresses {
		if strings.TrimSpace(email.Address) != "" {
			emails = append(emails, email.Address)
		}
	}
	return strings.Join(emails, ",")
}
