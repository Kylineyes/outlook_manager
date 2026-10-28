package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/cliconfig"
	"github.com/Kylineyes/outlook-manager/internal/update"
)

// stripGlobalUpdateFlag 移除全局 --no-update-check 参数。
func stripGlobalUpdateFlag(args []string) ([]string, bool) {
	result := make([]string, 0, len(args))
	disabled := false
	for _, arg := range args {
		if arg == "--no-update-check" {
			disabled = true
			continue
		}
		result = append(result, arg)
	}
	return result, disabled
}

func maybeStartupUpdateCheck(args []string, disabled bool) {
	if disabled || version == "dev" || os.Getenv("NO_UPDATE_CHECK") != "" || len(args) == 0 {
		return
	}
	switch strings.ToLower(args[0]) {
	case "update", "version", "--version", "-v", "help", "--help", "-h", "completion":
		return
	}
	settings := cliconfig.Load()
	if !settings.AutoUpdateCheck {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := update.StartupCheck(ctx, version, update.DefaultStartupCheckInterval)
	if err != nil || result == nil || !result.UpdateAvailable {
		return
	}
	fmt.Fprintf(os.Stderr, "\n提示: 发现新版本 %s（当前 %s）\n", result.Latest, result.Current)
	if result.Release != nil && result.Release.HTMLURL != "" {
		fmt.Fprintf(os.Stderr, "Release: %s\n", result.Release.HTMLURL)
	}
	fmt.Fprintln(os.Stderr, "运行 outlookctl update apply --yes 可升级")
}
