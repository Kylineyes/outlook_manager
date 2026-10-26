package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/audit"
	"github.com/Kylineyes/outlook-manager/internal/cliui"
	"github.com/Kylineyes/outlook-manager/internal/update"
)

func runUpdate(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl update <check|download|apply|rollback>")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl update <check|download|apply|rollback>")
	}
	switch args[0] {
	case "check":
		return runUpdateCheck(args[1:])
	case "download":
		return runUpdateDownload(args[1:])
	case "apply":
		return runUpdateApply(args[1:])
	case "rollback":
		return runUpdateRollback(args[1:])
	default:
		return fmt.Errorf("未知 update 子命令: %s", args[0])
	}
}

func updateContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Minute)
}

func currentRelease(ctx context.Context) (*update.Result, error) {
	result, err := update.Check(ctx, version)
	if err != nil {
		return nil, err
	}
	if result.Release == nil {
		return nil, errors.New("GitHub Release 响应缺少 release 数据")
	}
	return result, nil
}

func runUpdateCheck(args []string) error {
	fs := flag.NewFlagSet("update check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := updateContext()
	defer cancel()
	result, err := update.Check(ctx, version)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "当前版本: %s\n", result.Current)
	fmt.Fprintf(os.Stdout, "最新版本: %s\n", result.Latest)
	if result.IsDevelopment {
		fmt.Fprintln(os.Stdout, "当前是开发版本，无法比较更新。")
	} else if result.UpdateAvailable {
		fmt.Fprintln(os.Stdout, cliui.Colorize("状态: 发现新版本", cliui.Yellow))
	} else {
		fmt.Fprintln(os.Stdout, cliui.Colorize("状态: 已是最新版本", cliui.Green))
	}
	if result.Release != nil && result.Release.HTMLURL != "" {
		fmt.Fprintf(os.Stdout, "Release: %s\n", result.Release.HTMLURL)
	}
	return nil
}

func runUpdateDownload(args []string) error {
	fs := flag.NewFlagSet("update download", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outFlag := fs.String("out", "", "下载目录；不填时使用本地更新缓存")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	ctx, cancel := updateContext()
	defer cancel()
	release, err := currentRelease(ctx)
	if err != nil {
		return err
	}
	downloaded, err := update.DownloadRelease(ctx, release.Release, *outFlag)
	if err != nil {
		return err
	}
	audit.Log("update.download", map[string]any{"version": downloaded.Version, "asset": downloaded.AssetName, "checksumVerified": downloaded.ChecksumVerified, "signatureVerified": downloaded.SignatureVerified})
	if *jsonFlag {
		return printJSON(downloaded)
	}
	fmt.Fprintf(os.Stdout, "版本: %s\n", downloaded.Version)
	fmt.Fprintf(os.Stdout, "压缩包: %s\n", downloaded.ArchivePath)
	fmt.Fprintf(os.Stdout, "二进制: %s\n", downloaded.BinaryPath)
	fmt.Fprintln(os.Stdout, cliui.Colorize("SHA256 校验: 通过", cliui.Green))
	if downloaded.SignatureVerified {
		fmt.Fprintln(os.Stdout, cliui.Colorize("GPG 签名校验: 通过", cliui.Green))
	} else if downloaded.SignatureWarning != "" {
		fmt.Fprintf(os.Stdout, "GPG 签名: %s\n", downloaded.SignatureWarning)
	}
	return nil
}

func runUpdateApply(args []string) error {
	fs := flag.NewFlagSet("update apply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	yesFlag := fs.Bool("yes", false, "确认备份并替换当前二进制")
	forceFlag := fs.Bool("force", false, "即使版本相同也强制更新")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if !*yesFlag {
		return errors.New("替换当前二进制需要 --yes")
	}
	ctx, cancel := updateContext()
	defer cancel()
	release, err := currentRelease(ctx)
	if err != nil {
		return err
	}
	if !release.UpdateAvailable && !release.IsDevelopment && !*forceFlag {
		return errors.New("当前已是最新版本；如需强制更新请加 --force")
	}
	downloaded, err := update.DownloadRelease(ctx, release.Release, "")
	if err != nil {
		return err
	}
	applied, err := update.ApplyDownload(downloaded)
	if err != nil {
		return err
	}
	audit.Log("update.apply", map[string]any{"version": downloaded.Version, "exePath": applied.ExePath, "backupPath": applied.BackupPath})
	result := map[string]any{"success": true, "download": downloaded, "apply": applied}
	if *jsonFlag {
		return printJSON(result)
	}
	fmt.Fprintf(os.Stdout, "已准备更新: %s\n", downloaded.Version)
	fmt.Fprintf(os.Stdout, "新二进制: %s\n", downloaded.BinaryPath)
	fmt.Fprintf(os.Stdout, "备份: %s\n", applied.BackupPath)
	fmt.Fprintln(os.Stdout, cliui.Colorize(applied.Message, cliui.Green))
	return nil
}

func runUpdateRollback(args []string) error {
	fs := flag.NewFlagSet("update rollback", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	yesFlag := fs.Bool("yes", false, "确认使用备份回滚")
	jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
	if err := parseCommandFlags(fs, args); err != nil {
		return err
	}
	if !*yesFlag {
		return errors.New("回滚当前二进制需要 --yes")
	}
	applied, err := update.Rollback()
	if err != nil {
		return err
	}
	audit.Log("update.rollback", map[string]any{"exePath": applied.ExePath, "backupPath": applied.BackupPath})
	if *jsonFlag {
		return printJSON(applied)
	}
	fmt.Fprintf(os.Stdout, "回滚文件: %s\n", applied.BackupPath)
	fmt.Fprintln(os.Stdout, cliui.Colorize(applied.Message, cliui.Green))
	return nil
}
