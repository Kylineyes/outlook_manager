package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/audit"
	"github.com/Kylineyes/outlook-manager/internal/auth"
	"github.com/Kylineyes/outlook-manager/internal/cliui"
	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/secure"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// runDoctor 执行本地环境和账号诊断。
func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	checks := make([]doctorCheck, 0)
	add := func(name string, err error, message string) {
		if err != nil {
			checks = append(checks, doctorCheck{Name: name, OK: false, Message: err.Error()})
			return
		}
		checks = append(checks, doctorCheck{Name: name, OK: true, Message: message})
	}

	baseDir, err := store.EnsureBaseDir()
	add("数据目录", err, baseDir)
	if err == nil {
		testFile := baseDir + "\\.doctor-write-test"
		writeErr := os.WriteFile(testFile, []byte("ok"), 0o600)
		if writeErr == nil {
			_ = os.Remove(testFile)
		}
		add("目录写权限", writeErr, "可写")
	}

	cfg, err := store.LoadConfig()
	add("配置文件", err, "加载成功")
	if err == nil {
		add("OAuth 配置", nil, fmt.Sprintf("clientId=%s authority=%s scopes=%d", cfg.ClientID, cfg.Authority, len(cfg.Scopes)))
	}

	add("安全存储", secure.Check(), secure.Backend())
	accounts, err := store.LoadAccounts()
	add("账号档案", err, fmt.Sprintf("账号数量=%d", len(accounts.Accounts)))
	if err == nil && len(accounts.Accounts) == 0 {
		add("默认账号", fmt.Errorf("未找到已登录账号"), "")
	}

	tokenPath, err := store.TokenCachePath()
	add("Token 缓存路径", err, tokenPath)
	if err == nil {
		info, statErr := os.Stat(tokenPath)
		if statErr != nil {
			add("Token 缓存文件", statErr, "")
		} else {
			add("Token 缓存文件", nil, fmt.Sprintf("大小=%d 字节", info.Size()))
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client, err := auth.NewClient()
	add("认证客户端", err, "初始化成功")
	if err == nil {
		accountList, listErr := client.Accounts(ctx)
		add("Token 刷新前置检查", listErr, fmt.Sprintf("可读账号=%d", len(accountList)))
		if listErr == nil && len(accountList) > 0 {
			selected, selectErr := auth.SelectAccount(accountList, "")
			add("默认账号选择", selectErr, selected.Username)
			if selectErr == nil {
				loginResult, refreshErr := client.AcquireTokenSilent(ctx, selected)
				add("Silent token 刷新", refreshErr, "成功")
				if refreshErr == nil {
					me, meErr := graph.GetMe(ctx, loginResult.AccessToken)
					message := ""
					if meErr == nil {
						message = fmt.Sprintf("%s (%s)", me.DisplayName, me.UserPrincipalName)
					}
					add("Graph /me", meErr, message)
				}
			}
		}
	}

	auditPath, err := store.AuditLogPath()
	add("审计日志路径", err, auditPath)
	if err == nil {
		audit.Log("doctor.check", map[string]any{"result": "ok"})
		info, statErr := os.Stat(auditPath)
		if statErr != nil {
			add("审计日志写入", statErr, "")
		} else {
			add("审计日志写入", nil, fmt.Sprintf("大小=%d 字节", info.Size()))
		}
	}

	if *jsonFlag {
		data, marshalErr := json.MarshalIndent(checks, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Fprintln(os.Stdout, string(data))
	} else {
		for _, check := range checks {
			status := cliui.Colorize("PASS", cliui.Green)
			if !check.OK {
				status = cliui.Colorize("FAIL", cliui.Red)
			}
			fmt.Fprintf(os.Stdout, "[%s] %s: %s\n", status, check.Name, check.Message)
		}
	}
	for _, check := range checks {
		if !check.OK {
			return fmt.Errorf("诊断发现 %d 个问题", countFailedChecks(checks))
		}
	}
	return nil
}

func countFailedChecks(checks []doctorCheck) int {
	count := 0
	for _, check := range checks {
		if !check.OK {
			count++
		}
	}
	return count
}
