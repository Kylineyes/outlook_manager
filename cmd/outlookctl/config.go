package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Kylineyes/outlook-manager/internal/cliconfig"
)

func runConfig(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl config <path|get|set|alias>")
		return nil
	}
	if len(args) == 0 {
		return errors.New("用法: outlookctl config <path|get|set|alias>")
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
		fmt.Fprintf(os.Stdout, "aliases=%d\n", len(settings.Aliases))
		return nil
	case "set":
		fs := flag.NewFlagSet("config set", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		outputFlag := fs.String("output", "", "输出格式: text/table/json/yaml/jsonl")
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
			if value != "text" && value != "table" && value != "json" && value != "yaml" && value != "jsonl" {
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
	case "alias":
		return runConfigAlias(args[1:])
	default:
		return fmt.Errorf("未知 config 子命令: %s", args[0])
	}
}

func runConfigAlias(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: outlookctl config alias <set|list|delete>")
	}
	settings := cliconfig.Load()
	switch args[0] {
	case "list":
		if len(settings.Aliases) == 0 {
			fmt.Fprintln(os.Stdout, "没有配置别名")
			return nil
		}
		names := make([]string, 0, len(settings.Aliases))
		for name := range settings.Aliases {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(os.Stdout, "%s -> %s\n", name, settings.Aliases[name])
		}
		return nil
	case "set":
		if len(args) < 3 {
			return errors.New("用法: outlookctl config alias set <名称> <命令...>")
		}
		name := strings.TrimSpace(args[1])
		command := strings.TrimSpace(strings.Join(args[2:], " "))
		if name == "" || strings.ContainsAny(name, " 	") {
			return errors.New("别名不能为空且不能包含空白")
		}
		if command == "" {
			return errors.New("别名命令不能为空")
		}
		settings.Aliases[name] = command
		if err := cliconfig.Save(settings); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "别名已保存: %s -> %s\n", name, command)
		return nil
	case "delete", "remove":
		if len(args) != 2 {
			return errors.New("用法: outlookctl config alias delete <名称>")
		}
		delete(settings.Aliases, args[1])
		if err := cliconfig.Save(settings); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "别名已删除: %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("未知 config alias 子命令: %s", args[0])
	}
}
