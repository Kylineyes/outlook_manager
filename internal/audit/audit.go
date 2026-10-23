// Package audit 负责写入本地操作审计日志。
package audit

import (
	"encoding/json"
	"os"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// Log 追加一条 JSON Lines 审计记录。
// 该函数是尽力而为的：日志写入失败不会中断主操作。
func Log(action string, fields map[string]any) {
	path, err := store.AuditLogPath()
	if err != nil {
		return
	}
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
