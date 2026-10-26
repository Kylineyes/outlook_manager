// Package cliui 提供终端颜色和表格输出。
package cliui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Kylineyes/outlook-manager/internal/cliconfig"
)

const (
	Green  = "32"
	Red    = "31"
	Yellow = "33"
	Cyan   = "36"
	Dim    = "2"
)

// Enabled 判断是否启用颜色输出。
func Enabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OUTLOOK_COLOR"))) {
	case "always", "true", "1":
		return true
	case "never", "false", "0":
		return false
	}
	color := strings.ToLower(strings.TrimSpace(cliconfig.Load().Color))
	if color == "always" {
		return true
	}
	if color == "never" {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

// Colorize 按需给文本加 ANSI 颜色。
func Colorize(text, code string) string {
	if !Enabled() || code == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// PrintTable 输出简单文本表格。
func PrintTable(w io.Writer, headers []string, rows [][]string) error {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = utf8.RuneCountInString(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			if length := utf8.RuneCountInString(strings.ReplaceAll(cell, "\n", " ")); length > widths[i] {
				widths[i] = length
			}
		}
	}
	writeRow := func(row []string) {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprint(w, cell)
			if i < len(row)-1 {
				padding := widths[i] - utf8.RuneCountInString(strings.ReplaceAll(cell, "\n", " "))
				if padding > 0 {
					fmt.Fprint(w, strings.Repeat(" ", padding))
				}
			}
		}
		fmt.Fprintln(w)
	}
	writeRow(headers)
	separator := make([]string, len(headers))
	for i := range separator {
		separator[i] = strings.Repeat("-", widths[i])
	}
	writeRow(separator)
	for _, row := range rows {
		writeRow(row)
	}
	return nil
}
