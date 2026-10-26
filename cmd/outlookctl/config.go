package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Kylineyes/outlook-manager/internal/cliconfig"
)

func runConfig(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl config <path|get|set>")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl config <path|get|set>")
	}
	switch args[0] {
	case "path":
		path, err := cliconfig.Path()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, path)
		return nil
	case "get":
		fs := flag.NewFlagSet("config get", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
		if err := parseCommandFlags(fs, args[1:]); err != nil {
			return err
		}
		settings := cliconfig.Load()
		if *jsonFlag {
			return printJSON(settings)
		}
		fmt.Fprintf(os.Stdout, "output=%s\n", settings.Output)
		fmt.Fprintf(os.Stdout, "color=%s\n", settings.Color)
		fmt.Fprintf(os.Stdout, "autoUpdateCheck=%v\n", settings.AutoUpdateCheck)
		return nil
	case "set":
		fs := flag.NewFlagSet("config set", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		outputFlag := fs.String("output", "", "输出格式: text/table/json")
		colorFlag := fs.String("color", "", "颜色: auto/always/never")
		autoUpdateFlag := fs.Bool("auto-update-check", false, "是否在启动时检查更新")
		jsonFlag := fs.Bool("json", false, "以 JSON 格式输出")
		if err := parseCommandFlags(fs, args[1:]); err != nil {
			return err
		}
		settings := cliconfig.Load()
		visited := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
		if visited["output"] {
			value := strings.ToLower(strings.TrimSpace(*outputFlag))
			if value != "text" && value != "table" && value != "json" {
				return fmt.Errorf("不支持的输出格式: %s", *outputFlag)
			}
			settings.Output = value
		}
		if visited["color"] {
			value := strings.ToLower(strings.TrimSpace(*colorFlag))
			if value != "auto" && value != "always" && value != "never" {
				return fmt.Errorf("不支持的颜色模式: %s", *colorFlag)
			}
			settings.Color = value
		}
		if visited["auto-update-check"] {
			settings.AutoUpdateCheck = *autoUpdateFlag
		}
		if err := cliconfig.Save(settings); err != nil {
			return err
		}
		if *jsonFlag {
			return printJSON(settings)
		}
		fmt.Fprintln(os.Stdout, "CLI 配置已更新")
		fmt.Fprintf(os.Stdout, "output=%s\n", settings.Output)
		fmt.Fprintf(os.Stdout, "color=%s\n", settings.Color)
		fmt.Fprintf(os.Stdout, "autoUpdateCheck=%v\n", settings.AutoUpdateCheck)
		return nil
	default:
		return fmt.Errorf("未知 config 子命令: %s", args[0])
	}
}
