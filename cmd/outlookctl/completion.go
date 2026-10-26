package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var completionTop = []string{"auth", "account", "audit", "mailbox", "doctor", "update", "config", "sync", "search", "mail", "calendar", "contacts", "completion", "ui", "version", "help"}

var completionGroups = map[string][]string{
	"auth":     {"login", "status"},
	"account":  {"list", "use"},
	"audit":    {"list", "export", "stats", "verify", "config", "cleanup"},
	"mailbox":  {"settings", "auto-reply", "timezone"},
	"update":   {"check", "download", "apply", "rollback"},
	"config":   {"path", "get", "set"},
	"sync":     {"run", "status", "reset"},
	"mail":     {"list", "search", "query", "view", "send", "read", "mark-read", "mark-unread", "folders", "folder-tree", "move", "delete", "permanent-delete", "attachments", "attachment-save", "export-eml", "import-eml", "reply", "reply-all", "forward", "draft-create", "drafts", "draft-send", "draft-update", "bulk-move", "bulk-delete", "bulk-mark-read", "bulk-mark-unread", "bulk-flag", "bulk-unflag", "bulk-importance", "bulk-category-add", "bulk-category-remove", "rules", "rule-create", "rule-update", "rule-apply", "rule-delete", "master-categories"},
	"calendar": {"list", "calendars", "attachments", "attachment-add", "attachment-get", "attachment-delete", "attachment-replace", "attachment-bulk-delete", "export-ics", "import-ics", "series-info", "series-update", "series-delete", "freebusy", "find-time", "create", "update", "delete", "cancel", "respond", "forward"},
	"contacts": {"list", "search", "folders", "folder-create", "folder-rename", "folder-delete", "create", "update", "delete", "permanent-delete", "photo-get", "photo-set", "move", "copy", "export", "import", "bulk-delete", "duplicates"},
}

func runCompletion(args []string) error {
	if len(args) > 0 && helpRequested(args[:1]) {
		fmt.Fprintln(os.Stdout, "用法: outlookctl completion <powershell|bash|zsh|fish>")
		return nil
	}
	if len(args) != 1 {
		return errors.New("用法: outlookctl completion <powershell|bash|zsh|fish>")
	}
	shell := strings.ToLower(strings.TrimSpace(args[0]))
	switch shell {
	case "powershell", "pwsh":
		fmt.Fprint(os.Stdout, powershellCompletion())
	case "bash":
		fmt.Fprint(os.Stdout, bashCompletion())
	case "zsh":
		fmt.Fprint(os.Stdout, zshCompletion())
	case "fish":
		fmt.Fprint(os.Stdout, fishCompletion())
	default:
		return fmt.Errorf("不支持的 shell: %s", shell)
	}
	return nil
}

func quotedWords(words []string) string {
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		quoted = append(quoted, "'"+word+"'")
	}
	return strings.Join(quoted, " ")
}

func bashCompletion() string {
	var builder strings.Builder
	builder.WriteString("_outlookctl() {\n")
	builder.WriteString("  local cur\n")
	builder.WriteString("  cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	builder.WriteString("  if [ \"$COMP_CWORD\" -eq 1 ]; then\n")
	builder.WriteString("	COMPREPLY=( $(compgen -W \"" + strings.Join(completionTop, " ") + "\" -- \"$cur\") )\n")
	builder.WriteString("	return\n")
	builder.WriteString("  fi\n")
	builder.WriteString("  case \"${COMP_WORDS[1]}\" in\n")
	for group, subs := range completionGroups {
		builder.WriteString("	" + group + ") COMPREPLY=( $(compgen -W \"" + strings.Join(subs, " ") + "\" -- \"$cur\") );;\n")
	}
	builder.WriteString("  esac\n")
	builder.WriteString("}\n")
	builder.WriteString("complete -F _outlookctl outlookctl\n")
	return builder.String()
}

func zshCompletion() string {
	var builder strings.Builder
	builder.WriteString("#compdef outlookctl\n\n")
	builder.WriteString("_outlookctl() {\n")
	builder.WriteString("  local -a commands\n")
	builder.WriteString("  commands=(" + quotedWords(completionTop) + ")\n")
	builder.WriteString("  if (( CURRENT == 2 )); then\n")
	builder.WriteString("	_describe 'command' commands\n")
	builder.WriteString("	return\n")
	builder.WriteString("  fi\n")
	builder.WriteString("  case $words[2] in\n")
	for group, subs := range completionGroups {
		builder.WriteString("	" + group + ") _values 'subcommand' " + quotedWords(subs) + " ;;\n")
	}
	builder.WriteString("  esac\n")
	builder.WriteString("}\n")
	builder.WriteString("_outlookctl \"$@\"\n")
	return builder.String()
}

func fishCompletion() string {
	var builder strings.Builder
	for _, command := range completionTop {
		builder.WriteString("complete -c outlookctl -f -n '__fish_use_subcommand' -a '" + command + "'\n")
	}
	for group, subs := range completionGroups {
		for _, sub := range subs {
			builder.WriteString("complete -c outlookctl -f -n '__fish_seen_subcommand_from " + group + "' -a '" + sub + "'\n")
		}
	}
	return builder.String()
}

func powershellCompletion() string {
	commands := strings.Join(completionTop, "','")
	return "Register-ArgumentCompleter -Native -CommandName outlookctl -ScriptBlock {\n" +
		"  param($wordToComplete, $commandAst, $cursorPosition)\n" +
		"  $commands = @('" + commands + "')\n" +
		"  $commands | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n" +
		"	[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)\n" +
		"  }\n" +
		"}\n"
}
