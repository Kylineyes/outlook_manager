// Package localindex 使用 SQLite + FTS5 提供多账号本地索引。
package localindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

const schemaVersion = 2

// Store 表示本地 SQLite 索引。
type Store struct {
	db   *sql.DB
	path string
}

// SyncOptions 控制同步范围。
type SyncOptions struct {
	Messages bool
	Contacts bool
	Events   bool
	Max      int
}

// SyncReport 表示同步结果。
type SyncReport struct {
	Account     string    `json:"account"`
	Messages    int       `json:"messages"`
	Contacts    int       `json:"contacts"`
	Events      int       `json:"events"`
	MessageMode string    `json:"messageMode,omitempty"`
	ContactMode string    `json:"contactMode,omitempty"`
	EventMode   string    `json:"eventMode,omitempty"`
	Warnings    []string  `json:"warnings,omitempty"`
	SyncedAt    time.Time `json:"syncedAt"`
}

// AccountStats 表示单账号索引统计。
type AccountStats struct {
	Account      string    `json:"account"`
	SyncedAt     time.Time `json:"syncedAt"`
	Messages     int       `json:"messages"`
	Contacts     int       `json:"contacts"`
	Events       int       `json:"events"`
	MessageDelta bool      `json:"messageDelta"`
	ContactDelta bool      `json:"contactDelta"`
	EventDelta   bool      `json:"eventDelta"`
}

// Stats 表示本地索引统计。
type Stats struct {
	Version       int            `json:"version"`
	Accounts      []AccountStats `json:"accounts"`
	TotalMessages int            `json:"totalMessages"`
	TotalContacts int            `json:"totalContacts"`
	TotalEvents   int            `json:"totalEvents"`
}

// Hit 表示本地搜索结果。
type Hit struct {
	Account   string    `json:"account"`
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle,omitempty"`
	Snippet   string    `json:"snippet,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

type accountMeta struct {
	Account      string
	SyncedAt     time.Time
	MessageDelta string
	ContactDelta string
	EventDelta   string
}

type legacySnapshot struct {
	Version      int                   `json:"version"`
	SyncedAt     time.Time             `json:"syncedAt"`
	Messages     []graph.Message       `json:"messages"`
	Contacts     []graph.Contact       `json:"contacts"`
	Events       []graph.CalendarEvent `json:"events"`
	MessageDelta string                `json:"messageDelta,omitempty"`
	ContactDelta string                `json:"contactDelta,omitempty"`
	EventDelta   string                `json:"eventDelta,omitempty"`
}

// IndexPath 返回 SQLite 索引文件路径。
func IndexPath() (string, error) {
	dir, err := store.EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.db"), nil
}

func legacyJSONPath() (string, error) {
	dir, err := store.EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.json"), nil
}

// Open 打开并初始化本地索引。
func Open() (*Store, error) {
	path, err := IndexPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建索引目录失败: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 索引失败: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close 关闭索引。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) initialize() error {
	statements := []string{
		"PRAGMA page_size=8192",
		"PRAGMA auto_vacuum=INCREMENTAL",
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS accounts (account TEXT PRIMARY KEY, synced_at TEXT, message_delta TEXT, contact_delta TEXT, event_delta TEXT)`,
		`CREATE TABLE IF NOT EXISTS messages (account TEXT NOT NULL, id TEXT NOT NULL, subject TEXT, received_at TEXT, is_read INTEGER, has_attachments INTEGER, from_address TEXT, body_preview TEXT, data BLOB NOT NULL, PRIMARY KEY(account, id))`,
		`CREATE INDEX IF NOT EXISTS idx_messages_account_received ON messages(account, received_at DESC)`,
		`CREATE TABLE IF NOT EXISTS contacts (account TEXT NOT NULL, id TEXT NOT NULL, display_name TEXT, emails TEXT, phones TEXT, company TEXT, data BLOB NOT NULL, PRIMARY KEY(account, id))`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_account_name ON contacts(account, display_name)`,
		`CREATE TABLE IF NOT EXISTS events (account TEXT NOT NULL, id TEXT NOT NULL, subject TEXT, start_at TEXT, location TEXT, body_preview TEXT, data BLOB NOT NULL, PRIMARY KEY(account, id))`,
		`CREATE INDEX IF NOT EXISTS idx_events_account_start ON events(account, start_at)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(account UNINDEXED, kind UNINDEXED, id UNINDEXED, title, subtitle, body, timestamp UNINDEXED, tokenize='unicode61')`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("初始化 SQLite 索引失败 (%s): %w", statement, err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprintf("%d", schemaVersion)); err != nil {
		return fmt.Errorf("写入索引版本失败: %w", err)
	}
	return s.migrateLegacyJSON()
}

// Sync 同步指定账号的邮件、联系人和日历。
func Sync(ctx context.Context, accessToken string, options SyncOptions, account string) (*SyncReport, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, fmt.Errorf("同步账号不能为空")
	}
	if options.Max <= 0 {
		options.Max = 1000
	}
	store, err := Open()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	meta, err := store.loadAccountMeta(account)
	if err != nil {
		return nil, err
	}
	report := &SyncReport{Account: account}
	if options.Messages {
		mode, warning := store.syncMessages(ctx, accessToken, account, &meta, options.Max)
		report.MessageMode = mode
		if warning != "" {
			report.Warnings = append(report.Warnings, warning)
		}
	}
	if options.Contacts {
		mode, warning := store.syncContacts(ctx, accessToken, account, &meta, options.Max)
		report.ContactMode = mode
		if warning != "" {
			report.Warnings = append(report.Warnings, warning)
		}
	}
	if options.Events {
		mode, warning := store.syncEvents(ctx, accessToken, account, &meta, options.Max)
		report.EventMode = mode
		if warning != "" {
			report.Warnings = append(report.Warnings, warning)
		}
	}
	report.SyncedAt = time.Now().UTC()
	meta.SyncedAt = report.SyncedAt
	if err := store.saveAccountMeta(meta); err != nil {
		return report, err
	}
	counts, err := store.accountCounts(account)
	if err != nil {
		return report, err
	}
	report.Messages, report.Contacts, report.Events = counts[0], counts[1], counts[2]
	return report, nil
}

func (s *Store) syncMessages(ctx context.Context, accessToken, account string, meta *accountMeta, max int) (string, string) {
	result, err := graph.DeltaMessages(ctx, accessToken, meta.MessageDelta, max)
	if err == nil {
		if applyErr := s.applyMessageDelta(account, result.Items, result.Removed); applyErr != nil {
			return "failed", applyErr.Error()
		}
		meta.MessageDelta = result.DeltaLink
		return "delta", ""
	}
	items, fullErr := graph.ListAllMessagesAll(ctx, accessToken, max)
	if fullErr != nil {
		return "failed", fmt.Sprintf("邮件同步失败: delta=%v; full=%v", err, fullErr)
	}
	if replaceErr := s.replaceMessages(account, items); replaceErr != nil {
		return "failed", replaceErr.Error()
	}
	meta.MessageDelta = ""
	return "full-fallback", fmt.Sprintf("邮件 delta 不可用，已回退全量: %v", err)
}

func (s *Store) syncContacts(ctx context.Context, accessToken, account string, meta *accountMeta, max int) (string, string) {
	result, err := graph.DeltaContacts(ctx, accessToken, meta.ContactDelta, max)
	if err == nil {
		if applyErr := s.applyContactDelta(account, result.Items, result.Removed); applyErr != nil {
			return "failed", applyErr.Error()
		}
		meta.ContactDelta = result.DeltaLink
		return "delta", ""
	}
	items, fullErr := graph.ListContactsInFolderAll(ctx, accessToken, "", max)
	if fullErr != nil {
		return "failed", fmt.Sprintf("联系人同步失败: delta=%v; full=%v", err, fullErr)
	}
	if replaceErr := s.replaceContacts(account, items); replaceErr != nil {
		return "failed", replaceErr.Error()
	}
	meta.ContactDelta = ""
	return "full-fallback", fmt.Sprintf("联系人 delta 不可用，已回退全量: %v", err)
}

func (s *Store) syncEvents(ctx context.Context, accessToken, account string, meta *accountMeta, max int) (string, string) {
	result, err := graph.DeltaEvents(ctx, accessToken, meta.EventDelta, max)
	if err == nil {
		if applyErr := s.applyEventDelta(account, result.Items, result.Removed); applyErr != nil {
			return "failed", applyErr.Error()
		}
		meta.EventDelta = result.DeltaLink
		return "delta", ""
	}
	items, fullErr := graph.ListCalendarEventsAll(ctx, accessToken, "", max)
	if fullErr != nil {
		return "failed", fmt.Sprintf("日历同步失败: delta=%v; full=%v", err, fullErr)
	}
	if replaceErr := s.replaceEvents(account, items); replaceErr != nil {
		return "failed", replaceErr.Error()
	}
	meta.EventDelta = ""
	return "full-fallback", fmt.Sprintf("日历 delta 不可用，已回退全量: %v", err)
}

func (s *Store) loadAccountMeta(account string) (accountMeta, error) {
	meta := accountMeta{Account: account}
	var syncedAt string
	row := s.db.QueryRow(`SELECT COALESCE(synced_at,''), COALESCE(message_delta,''), COALESCE(contact_delta,''), COALESCE(event_delta,'') FROM accounts WHERE account = ?`, account)
	if err := row.Scan(&syncedAt, &meta.MessageDelta, &meta.ContactDelta, &meta.EventDelta); err != nil {
		if err != sql.ErrNoRows {
			return meta, fmt.Errorf("读取账号索引元数据失败: %w", err)
		}
		if _, err := s.db.Exec(`INSERT INTO accounts(account) VALUES(?)`, account); err != nil {
			return meta, fmt.Errorf("创建账号索引元数据失败: %w", err)
		}
	}
	if syncedAt != "" {
		meta.SyncedAt, _ = time.Parse(time.RFC3339, syncedAt)
	}
	return meta, nil
}

func (s *Store) saveAccountMeta(meta accountMeta) error {
	_, err := s.db.Exec(`INSERT INTO accounts(account, synced_at, message_delta, contact_delta, event_delta) VALUES(?,?,?,?,?) ON CONFLICT(account) DO UPDATE SET synced_at=excluded.synced_at, message_delta=excluded.message_delta, contact_delta=excluded.contact_delta, event_delta=excluded.event_delta`,
		meta.Account, meta.SyncedAt.UTC().Format(time.RFC3339), meta.MessageDelta, meta.ContactDelta, meta.EventDelta)
	if err != nil {
		return fmt.Errorf("保存账号索引元数据失败: %w", err)
	}
	return nil
}

func (s *Store) accountCounts(account string) ([3]int, error) {
	var counts [3]int
	queries := []string{
		`SELECT COUNT(*) FROM messages WHERE account = ?`,
		`SELECT COUNT(*) FROM contacts WHERE account = ?`,
		`SELECT COUNT(*) FROM events WHERE account = ?`,
	}
	for i, query := range queries {
		if err := s.db.QueryRow(query, account).Scan(&counts[i]); err != nil {
			return counts, err
		}
	}
	return counts, nil
}

func ensureAccountTx(tx *sql.Tx, account string) error {
	_, err := tx.Exec(`INSERT INTO accounts(account) VALUES(?) ON CONFLICT(account) DO NOTHING`, account)
	return err
}

func (s *Store) applyMessageDelta(account string, messages []graph.Message, removed []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureAccountTx(tx, account); err != nil {
		return err
	}
	for _, message := range messages {
		if err := insertMessageTx(tx, account, message); err != nil {
			return err
		}
	}
	for _, id := range removed {
		if err := deleteMessageTx(tx, account, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) replaceMessages(account string, messages []graph.Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureAccountTx(tx, account); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE account = ?`, account); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM search_fts WHERE account = ? AND kind = 'mail'`, account); err != nil {
		return err
	}
	for _, message := range messages {
		if err := insertMessageTx(tx, account, message); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) applyContactDelta(account string, contacts []graph.Contact, removed []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureAccountTx(tx, account); err != nil {
		return err
	}
	for _, contact := range contacts {
		if err := insertContactTx(tx, account, contact); err != nil {
			return err
		}
	}
	for _, id := range removed {
		if err := deleteContactTx(tx, account, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) replaceContacts(account string, contacts []graph.Contact) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureAccountTx(tx, account); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM contacts WHERE account = ?`, account); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM search_fts WHERE account = ? AND kind = 'contact'`, account); err != nil {
		return err
	}
	for _, contact := range contacts {
		if err := insertContactTx(tx, account, contact); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) applyEventDelta(account string, events []graph.CalendarEvent, removed []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureAccountTx(tx, account); err != nil {
		return err
	}
	for _, event := range events {
		if err := insertEventTx(tx, account, event); err != nil {
			return err
		}
	}
	for _, id := range removed {
		if err := deleteEventTx(tx, account, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) replaceEvents(account string, events []graph.CalendarEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureAccountTx(tx, account); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM events WHERE account = ?`, account); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM search_fts WHERE account = ? AND kind = 'event'`, account); err != nil {
		return err
	}
	for _, event := range events {
		if err := insertEventTx(tx, account, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertMessageTx(tx *sql.Tx, account string, message graph.Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	from := message.From.EmailAddress.Address
	if from == "" {
		from = message.From.EmailAddress.Name
	}
	if _, err := tx.Exec(`INSERT INTO messages(account,id,subject,received_at,is_read,has_attachments,from_address,body_preview,data) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(account,id) DO UPDATE SET subject=excluded.subject, received_at=excluded.received_at, is_read=excluded.is_read, has_attachments=excluded.has_attachments, from_address=excluded.from_address, body_preview=excluded.body_preview, data=excluded.data`,
		account, message.ID, message.Subject, message.ReceivedDateTime.UTC().Format(time.RFC3339), boolInt(message.IsRead), boolInt(message.HasAttachments), from, message.BodyPreview, data); err != nil {
		return err
	}
	if err := deleteSearchTx(tx, account, "mail", message.ID); err != nil {
		return err
	}
	return insertSearchTx(tx, account, "mail", message.ID, message.Subject, from, message.BodyPreview, message.ReceivedDateTime)
}

func insertContactTx(tx *sql.Tx, account string, contact graph.Contact) error {
	data, err := json.Marshal(contact)
	if err != nil {
		return err
	}
	emails := make([]string, 0, len(contact.EmailAddresses))
	for _, email := range contact.EmailAddresses {
		emails = append(emails, email.Address)
	}
	phones := append([]string(nil), contact.BusinessPhones...)
	if contact.MobilePhone != "" {
		phones = append(phones, contact.MobilePhone)
	}
	if _, err := tx.Exec(`INSERT INTO contacts(account,id,display_name,emails,phones,company,data) VALUES(?,?,?,?,?,?,?) ON CONFLICT(account,id) DO UPDATE SET display_name=excluded.display_name, emails=excluded.emails, phones=excluded.phones, company=excluded.company, data=excluded.data`,
		account, contact.ID, contact.DisplayName, strings.Join(emails, "\n"), strings.Join(phones, "\n"), contact.CompanyName, data); err != nil {
		return err
	}
	if err := deleteSearchTx(tx, account, "contact", contact.ID); err != nil {
		return err
	}
	body := strings.Join(append([]string{contact.CompanyName, contact.JobTitle, contact.PersonalNotes, contact.Department, contact.OfficeLocation}, phones...), "\n")
	return insertSearchTx(tx, account, "contact", contact.ID, contact.DisplayName, strings.Join(emails, ", "), body, time.Time{})
}

func insertEventTx(tx *sql.Tx, account string, event graph.CalendarEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	start, _ := time.Parse(time.RFC3339, event.Start.DateTime)
	if start.IsZero() {
		start, _ = time.Parse("2006-01-02T15:04:05", event.Start.DateTime)
	}
	if _, err := tx.Exec(`INSERT INTO events(account,id,subject,start_at,location,body_preview,data) VALUES(?,?,?,?,?,?,?) ON CONFLICT(account,id) DO UPDATE SET subject=excluded.subject, start_at=excluded.start_at, location=excluded.location, body_preview=excluded.body_preview, data=excluded.data`,
		account, event.ID, event.Subject, start.UTC().Format(time.RFC3339), event.Location.DisplayName, event.BodyPreview, data); err != nil {
		return err
	}
	if err := deleteSearchTx(tx, account, "event", event.ID); err != nil {
		return err
	}
	return insertSearchTx(tx, account, "event", event.ID, event.Subject, event.Location.DisplayName, event.BodyPreview, start)
}

func deleteMessageTx(tx *sql.Tx, account, id string) error {
	if _, err := tx.Exec(`DELETE FROM messages WHERE account = ? AND id = ?`, account, id); err != nil {
		return err
	}
	return deleteSearchTx(tx, account, "mail", id)
}

func deleteContactTx(tx *sql.Tx, account, id string) error {
	if _, err := tx.Exec(`DELETE FROM contacts WHERE account = ? AND id = ?`, account, id); err != nil {
		return err
	}
	return deleteSearchTx(tx, account, "contact", id)
}

func deleteEventTx(tx *sql.Tx, account, id string) error {
	if _, err := tx.Exec(`DELETE FROM events WHERE account = ? AND id = ?`, account, id); err != nil {
		return err
	}
	return deleteSearchTx(tx, account, "event", id)
}

func deleteSearchTx(tx *sql.Tx, account, kind, id string) error {
	_, err := tx.Exec(`DELETE FROM search_fts WHERE account = ? AND kind = ? AND id = ?`, account, kind, id)
	return err
}

func insertSearchTx(tx *sql.Tx, account, kind, id, title, subtitle, body string, timestamp time.Time) error {
	_, err := tx.Exec(`INSERT INTO search_fts(account,kind,id,title,subtitle,body,timestamp) VALUES(?,?,?,?,?,?,?)`, account, kind, id, title, subtitle, body, timestamp.UTC().Format(time.RFC3339))
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// LoadStats 返回索引统计。
func LoadStats(account string) (*Stats, error) {
	store, err := Open()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.Stats(account)
}

func (s *Store) Stats(account string) (*Stats, error) {
	query := `SELECT account, COALESCE(synced_at,''), COALESCE(message_delta,''), COALESCE(contact_delta,''), COALESCE(event_delta,'') FROM accounts`
	args := []any{}
	if strings.TrimSpace(account) != "" {
		query += ` WHERE account = ?`
		args = append(args, strings.TrimSpace(account))
	}
	query += ` ORDER BY account`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := &Stats{Version: schemaVersion, Accounts: []AccountStats{}}
	for rows.Next() {
		var item AccountStats
		var syncedAt, messageDelta, contactDelta, eventDelta string
		if err := rows.Scan(&item.Account, &syncedAt, &messageDelta, &contactDelta, &eventDelta); err != nil {
			rows.Close()
			return nil, err
		}
		item.SyncedAt, _ = time.Parse(time.RFC3339, syncedAt)
		item.MessageDelta = messageDelta != ""
		item.ContactDelta = contactDelta != ""
		item.EventDelta = eventDelta != ""
		stats.Accounts = append(stats.Accounts, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range stats.Accounts {
		counts, err := s.accountCounts(stats.Accounts[i].Account)
		if err != nil {
			return nil, err
		}
		stats.Accounts[i].Messages = counts[0]
		stats.Accounts[i].Contacts = counts[1]
		stats.Accounts[i].Events = counts[2]
		stats.TotalMessages += counts[0]
		stats.TotalContacts += counts[1]
		stats.TotalEvents += counts[2]
	}
	return stats, nil
}

// Search 搜索本地索引。
func Search(account, query, kind string, limit int) ([]Hit, error) {
	store, err := Open()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.Search(account, query, kind, limit)
}

func (s *Store) Search(account, query, kind string, limit int) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	account = strings.TrimSpace(account)
	kind = strings.ToLower(strings.TrimSpace(kind))
	hits := make([]Hit, 0)
	if ftsQuery := buildFTSQuery(query); ftsQuery != "" {
		sqlText := `SELECT account, kind, id, title, COALESCE(subtitle,''), COALESCE(body,''), COALESCE(timestamp,'') FROM search_fts WHERE search_fts MATCH ?`
		args := []any{ftsQuery}
		if account != "" {
			sqlText += ` AND account = ?`
			args = append(args, account)
		}
		if kinds := kindFilter(kind); len(kinds) > 0 {
			sqlText += ` AND kind IN (` + placeholders(len(kinds)) + `)`
			for _, value := range kinds {
				args = append(args, value)
			}
		}
		sqlText += ` ORDER BY timestamp DESC LIMIT ?`
		args = append(args, limit*2)
		rows, err := s.db.Query(sqlText, args...)
		if err == nil {
			ftsHits, scanErr := scanHits(rows)
			rows.Close()
			if scanErr == nil {
				hits = append(hits, ftsHits...)
			}
		}
	}
	if len(hits) < limit {
		like := "%" + escapeLike(query) + "%"
		sqlText := `SELECT account, kind, id, title, COALESCE(subtitle,''), COALESCE(body,''), COALESCE(timestamp,'') FROM search_fts WHERE (title LIKE ? ESCAPE '\' OR subtitle LIKE ? ESCAPE '\' OR body LIKE ? ESCAPE '\')`
		args := []any{like, like, like}
		if account != "" {
			sqlText += ` AND account = ?`
			args = append(args, account)
		}
		if kinds := kindFilter(kind); len(kinds) > 0 {
			sqlText += ` AND kind IN (` + placeholders(len(kinds)) + `)`
			for _, value := range kinds {
				args = append(args, value)
			}
		}
		sqlText += ` ORDER BY timestamp DESC LIMIT ?`
		args = append(args, limit*2)
		rows, err := s.db.Query(sqlText, args...)
		if err != nil {
			return nil, err
		}
		likeHits, err := scanHits(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		hits = append(hits, likeHits...)
	}
	hits = dedupeHits(hits)
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Timestamp.After(hits[j].Timestamp) })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func scanHits(rows *sql.Rows) ([]Hit, error) {
	hits := make([]Hit, 0)
	for rows.Next() {
		var hit Hit
		var timestamp string
		if err := rows.Scan(&hit.Account, &hit.Type, &hit.ID, &hit.Title, &hit.Subtitle, &hit.Snippet, &timestamp); err != nil {
			return nil, err
		}
		hit.Timestamp, _ = time.Parse(time.RFC3339, timestamp)
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

func dedupeHits(hits []Hit) []Hit {
	seen := map[string]bool{}
	result := make([]Hit, 0, len(hits))
	for _, hit := range hits {
		key := hit.Account + "\x00" + hit.Type + "\x00" + hit.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, hit)
	}
	return result
}

func buildFTSQuery(query string) string {
	fields := strings.Fields(strings.TrimSpace(query))
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		field = strings.ReplaceAll(field, `"`, `""`)
		terms = append(terms, `"`+field+`"*`)
	}
	return strings.Join(terms, " AND ")
}

func kindFilter(kind string) []string {
	switch kind {
	case "mail", "contact", "event":
		return []string{kind}
	default:
		return nil
	}
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

// ResetAccount 删除指定账号的本地索引。
func ResetAccount(account string) error {
	store, err := Open()
	if err != nil {
		return err
	}
	defer store.Close()
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{`DELETE FROM messages WHERE account = ?`, `DELETE FROM contacts WHERE account = ?`, `DELETE FROM events WHERE account = ?`, `DELETE FROM search_fts WHERE account = ?`, `DELETE FROM accounts WHERE account = ?`} {
		if _, err := tx.Exec(query, account); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResetAll 删除整个 SQLite 索引。
func ResetAll() error {
	path, err := IndexPath()
	if err != nil {
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Compact 压缩并优化 SQLite 索引。
func Compact() error {
	store, err := Open()
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := store.db.Exec(`PRAGMA optimize`); err != nil {
		return err
	}
	if _, err := store.db.Exec(`VACUUM`); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateLegacyJSON() error {
	path, err := legacyJSONPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var snapshot legacySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil
	}
	var accounts int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts); err != nil {
		return err
	}
	if accounts > 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const account = "legacy"
	if _, err := tx.Exec(`INSERT INTO accounts(account, synced_at, message_delta, contact_delta, event_delta) VALUES(?,?,?,?,?)`, account, snapshot.SyncedAt.UTC().Format(time.RFC3339), snapshot.MessageDelta, snapshot.ContactDelta, snapshot.EventDelta); err != nil {
		return err
	}
	for _, message := range snapshot.Messages {
		if err := insertMessageTx(tx, account, message); err != nil {
			return err
		}
	}
	for _, contact := range snapshot.Contacts {
		if err := insertContactTx(tx, account, contact); err != nil {
			return err
		}
	}
	for _, event := range snapshot.Events {
		if err := insertEventTx(tx, account, event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = os.Rename(path, path+".migrated")
	return nil
}
