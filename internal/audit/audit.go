// Package audit 负责写入和查询本地操作审计日志。
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

var maxAuditLogSize int64 = 10 * 1024 * 1024

const (
	auditMaxBackups    = 5
	auditRetentionDays = 30
)

// Log 追加一条 JSON Lines 审计记录。
// 该函数是尽力而为的：日志写入失败不会中断主操作。
func Log(action string, fields map[string]any) {
	path, err := store.AuditLogPath()
	if err != nil {
		return
	}
	rotateIfNeeded(path)
	entry := map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339),
		"action": action,
	}
	for key, value := range fields {
		entry[key] = value
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(data, '\n'))
}

// Entries 读取最近的审计记录，可按 action 前缀过滤。
func Entries(limit int, actionPrefix string) ([]map[string]any, error) {
	path, err := store.AuditLogPath()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	paths := make([]string, 0)
	for _, file := range files {
		name := file.Name()
		if name == "audit.log" || (strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".log")) {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	sort.Strings(paths)
	entries := make([]map[string]any, 0)
	actionPrefix = strings.TrimSpace(actionPrefix)
	for _, filePath := range paths {
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			action, _ := entry["action"].(string)
			if actionPrefix != "" && !strings.HasPrefix(action, actionPrefix) {
				continue
			}
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, _ := entries[i]["time"].(string)
		right, _ := entries[j]["time"].(string)
		return left < right
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

func rotateIfNeeded(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxAuditLogSize {
		return
	}
	dir := filepath.Dir(path)
	base := "audit-" + time.Now().UTC().Format("20060102-150405")
	backup := filepath.Join(dir, base+".log")
	for counter := 1; ; counter++ {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			break
		}
		backup = filepath.Join(dir, base+"-"+time.Now().UTC().Format("150405")+"-"+itoa(counter)+".log")
	}
	if err := os.Rename(path, backup); err != nil {
		return
	}
	pruneAuditLogs(dir)
}

func pruneAuditLogs(dir string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type backupFile struct {
		path    string
		modTime time.Time
	}
	backups := make([]backupFile, 0)
	cutoff := time.Now().Add(-auditRetentionDays * 24 * time.Hour)
	for _, file := range files {
		name := file.Name()
		if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		filePath := filepath.Join(dir, name)
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filePath)
			continue
		}
		backups = append(backups, backupFile{path: filePath, modTime: info.ModTime()})
	}
	sort.SliceStable(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})
	for i := auditMaxBackups; i < len(backups); i++ {
		_ = os.Remove(backups[i].path)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
