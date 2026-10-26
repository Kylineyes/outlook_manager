// Package audit 负责写入、查询、导出和校验本地操作审计日志。
package audit

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

const (
	// DefaultMaxSizeBytes 是单个审计日志的默认大小上限。
	DefaultMaxSizeBytes int64 = 10 * 1024 * 1024
	// DefaultMaxBackups 是默认保留的历史文件数量。
	DefaultMaxBackups = 5
	// DefaultRetentionDays 是默认保留天数。
	DefaultRetentionDays = 30
)

var (
	logMu          sync.Mutex
	accountMu      sync.RWMutex
	currentAccount string
)

// Settings 表示审计日志保留和轮转配置。
type Settings struct {
	MaxSizeBytes  int64 `json:"maxSizeBytes"`
	MaxBackups    int   `json:"maxBackups"`
	RetentionDays int   `json:"retentionDays"`
}

// DefaultSettings 返回默认审计配置。
func DefaultSettings() Settings {
	return Settings{MaxSizeBytes: DefaultMaxSizeBytes, MaxBackups: DefaultMaxBackups, RetentionDays: DefaultRetentionDays}
}

// SettingsPath 返回审计配置文件路径。
func SettingsPath() (string, error) {
	dir, err := store.EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit-config.json"), nil
}

// LoadSettings 读取审计配置。
func LoadSettings() Settings {
	settings := DefaultSettings()
	path, err := SettingsPath()
	if err != nil {
		return settings
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	var file Settings
	if err := json.Unmarshal(data, &file); err != nil {
		return settings
	}
	if file.MaxSizeBytes > 0 {
		settings.MaxSizeBytes = file.MaxSizeBytes
	}
	if file.MaxBackups > 0 {
		settings.MaxBackups = file.MaxBackups
	}
	if file.RetentionDays > 0 {
		settings.RetentionDays = file.RetentionDays
	}
	return settings
}

// SaveSettings 保存审计配置。
func SaveSettings(settings Settings) error {
	if settings.MaxSizeBytes <= 0 {
		settings.MaxSizeBytes = DefaultMaxSizeBytes
	}
	if settings.MaxBackups <= 0 {
		settings.MaxBackups = DefaultMaxBackups
	}
	if settings.RetentionDays <= 0 {
		settings.RetentionDays = DefaultRetentionDays
	}
	path, err := SettingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化审计配置失败: %w", err)
	}
	return store.WriteFileAtomic(path, data, 0o600)
}

// SetAccount 设置当前审计上下文账号。
func SetAccount(account string) {
	accountMu.Lock()
	currentAccount = strings.TrimSpace(account)
	accountMu.Unlock()
}

// ClearAccount 清除当前审计上下文账号。
func ClearAccount() {
	SetAccount("")
}

func accountValue() string {
	accountMu.RLock()
	defer accountMu.RUnlock()
	return currentAccount
}

// Log 追加一条 JSON Lines 审计记录。
func Log(action string, fields map[string]any) {
	logMu.Lock()
	defer logMu.Unlock()
	path, err := store.AuditLogPath()
	if err != nil {
		return
	}
	settings := LoadSettings()
	rotateIfNeeded(path, settings)
	entry := map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339),
		"action": action,
	}
	for key, value := range fields {
		entry[key] = value
	}
	if _, exists := entry["account"]; !exists {
		if account := accountValue(); account != "" {
			entry["account"] = account
		}
	}
	if _, exists := entry["hash"]; !exists {
		entry["prevHash"] = previousHash(path)
		entry["hash"] = entryHash(entry)
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

// QueryOptions 表示审计查询条件。
type QueryOptions struct {
	Limit        int
	ActionPrefix string
	Account      string
	Query        string
	Since        time.Time
	Until        time.Time
}

// Entries 读取最近的审计记录，可按 action 前缀过滤。
func Entries(limit int, actionPrefix string) ([]map[string]any, error) {
	return Query(QueryOptions{Limit: limit, ActionPrefix: actionPrefix})
}

// Query 按条件读取审计记录。
func Query(options QueryOptions) ([]map[string]any, error) {
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
	actionPrefix := strings.TrimSpace(options.ActionPrefix)
	accountFilter := strings.ToLower(strings.TrimSpace(options.Account))
	query := strings.ToLower(strings.TrimSpace(options.Query))
	entries := make([]map[string]any, 0)
	for _, filePath := range paths {
		fileEntries, err := readFile(filePath)
		if err != nil {
			continue
		}
		for _, entry := range fileEntries {
			action, _ := entry["action"].(string)
			if actionPrefix != "" && !strings.HasPrefix(action, actionPrefix) {
				continue
			}
			account, _ := entry["account"].(string)
			if accountFilter != "" && !strings.EqualFold(account, accountFilter) {
				continue
			}
			entryTime, _ := entryTime(entry)
			if !options.Since.IsZero() && entryTime.Before(options.Since) {
				continue
			}
			if !options.Until.IsZero() && entryTime.After(options.Until) {
				continue
			}
			if query != "" {
				data, _ := json.Marshal(entry)
				if !strings.Contains(strings.ToLower(string(data)), query) {
					continue
				}
			}
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, _ := entries[i]["time"].(string)
		right, _ := entries[j]["time"].(string)
		return left < right
	})
	if options.Limit > 0 && len(entries) > options.Limit {
		entries = entries[len(entries)-options.Limit:]
	}
	return entries, nil
}

func readFile(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func entryTime(entry map[string]any) (time.Time, error) {
	value, _ := entry["time"].(string)
	return time.Parse(time.RFC3339, value)
}

// ExportJSON 把审计记录导出为 JSON。
func ExportJSON(w io.Writer, entries []map[string]any) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化审计导出失败: %w", err)
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// ExportCSV 把审计记录导出为 CSV。
func ExportCSV(w io.Writer, entries []map[string]any) error {
	columns := make(map[string]bool)
	for _, entry := range entries {
		for key := range entry {
			columns[key] = true
		}
	}
	headers := make([]string, 0, len(columns))
	for key := range columns {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	writer := csv.NewWriter(w)
	defer writer.Flush()
	if err := writer.Write(headers); err != nil {
		return err
	}
	for _, entry := range entries {
		row := make([]string, len(headers))
		for index, header := range headers {
			value, ok := entry[header]
			if !ok {
				continue
			}
			if text, ok := value.(string); ok {
				row[index] = text
			} else {
				data, _ := json.Marshal(value)
				row[index] = string(data)
			}
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	return writer.Error()
}

// Stats 表示审计统计结果。
type Stats struct {
	Total    int            `json:"total"`
	First    string         `json:"first,omitempty"`
	Last     string         `json:"last,omitempty"`
	Actions  map[string]int `json:"actions"`
	Accounts map[string]int `json:"accounts"`
}

// BuildStats 统计审计记录。
func BuildStats(entries []map[string]any) Stats {
	stats := Stats{Total: len(entries), Actions: map[string]int{}, Accounts: map[string]int{}}
	for _, entry := range entries {
		action, _ := entry["action"].(string)
		if action != "" {
			stats.Actions[action]++
		}
		account, _ := entry["account"].(string)
		if account != "" {
			stats.Accounts[account]++
		}
		timeText, _ := entry["time"].(string)
		if timeText != "" {
			if stats.First == "" || timeText < stats.First {
				stats.First = timeText
			}
			if stats.Last == "" || timeText > stats.Last {
				stats.Last = timeText
			}
		}
	}
	return stats
}

// VerifyResult 表示审计哈希链校验结果。
type VerifyResult struct {
	Files      int    `json:"files"`
	Checked    int    `json:"checked"`
	Legacy     int    `json:"legacy"`
	Valid      bool   `json:"valid"`
	BrokenFile string `json:"brokenFile,omitempty"`
	BrokenLine int    `json:"brokenLine,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Verify 校验审计日志哈希链。
func Verify() (VerifyResult, error) {
	path, err := store.AuditLogPath()
	if err != nil {
		return VerifyResult{}, err
	}
	dir := filepath.Dir(path)
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return VerifyResult{Valid: true}, nil
		}
		return VerifyResult{}, err
	}
	paths := make([]string, 0)
	for _, file := range files {
		name := file.Name()
		if name == "audit.log" || (strings.HasPrefix(name, "audit-") && strings.HasSuffix(name, ".log")) {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	sort.Strings(paths)
	result := VerifyResult{Valid: true, Files: len(paths)}
	for _, filePath := range paths {
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		previous := ""
		for lineNumber, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			hash, _ := entry["hash"].(string)
			if hash == "" {
				result.Legacy++
				continue
			}
			prevHash, _ := entry["prevHash"].(string)
			if prevHash != previous || entryHash(entry) != hash {
				result.Valid = false
				result.BrokenFile = filePath
				result.BrokenLine = lineNumber + 1
				result.Message = "审计哈希链断裂"
				return result, nil
			}
			previous = hash
			result.Checked++
		}
	}
	return result, nil
}

// CleanupResult 表示审计清理结果。
type CleanupResult struct {
	Removed int `json:"removed"`
}

// Cleanup 按配置清理历史审计日志。
func Cleanup(settings Settings) (CleanupResult, error) {
	path, err := store.AuditLogPath()
	if err != nil {
		return CleanupResult{}, err
	}
	rotateIfNeeded(path, settings)
	removed := pruneAuditLogs(filepath.Dir(path), settings)
	return CleanupResult{Removed: removed}, nil
}

func previousHash(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	offset := info.Size() - 8192
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil {
			hash, _ := entry["hash"].(string)
			return hash
		}
	}
	return ""
}

func entryHash(entry map[string]any) string {
	copyEntry := make(map[string]any, len(entry))
	for key, value := range entry {
		if key == "hash" {
			continue
		}
		copyEntry[key] = value
	}
	data, _ := json.Marshal(copyEntry)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func rotateIfNeeded(path string, settings Settings) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < settings.MaxSizeBytes {
		return
	}
	dir := filepath.Dir(path)
	base := "audit-" + time.Now().UTC().Format("20060102-150405")
	backup := filepath.Join(dir, base+".log")
	for counter := 1; ; counter++ {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			break
		}
		backup = filepath.Join(dir, base+"-"+time.Now().UTC().Format("150405")+"-"+strconv.Itoa(counter)+".log")
	}
	if err := os.Rename(path, backup); err != nil {
		return
	}
	pruneAuditLogs(dir, settings)
}

func pruneAuditLogs(dir string, settings Settings) int {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	type backupFile struct {
		path    string
		modTime time.Time
	}
	backups := make([]backupFile, 0)
	cutoff := time.Now().Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)
	removed := 0
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
			if os.Remove(filePath) == nil {
				removed++
			}
			continue
		}
		backups = append(backups, backupFile{path: filePath, modTime: info.ModTime()})
	}
	sort.SliceStable(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})
	for i := settings.MaxBackups; i < len(backups); i++ {
		if os.Remove(backups[i].path) == nil {
			removed++
		}
	}
	return removed
}
