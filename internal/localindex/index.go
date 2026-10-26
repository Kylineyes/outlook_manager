// Package localindex 提供本地邮件、联系人和日历索引及增量同步。
package localindex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
	"github.com/Kylineyes/outlook-manager/internal/store"
)

const indexVersion = 1

// Snapshot 表示本地索引快照。
type Snapshot struct {
	Version      int                   `json:"version"`
	SyncedAt     time.Time             `json:"syncedAt"`
	Messages     []graph.Message       `json:"messages"`
	Contacts     []graph.Contact       `json:"contacts"`
	Events       []graph.CalendarEvent `json:"events"`
	MessageDelta string                `json:"messageDelta,omitempty"`
	ContactDelta string                `json:"contactDelta,omitempty"`
	EventDelta   string                `json:"eventDelta,omitempty"`
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
	Messages    int       `json:"messages"`
	Contacts    int       `json:"contacts"`
	Events      int       `json:"events"`
	MessageMode string    `json:"messageMode,omitempty"`
	ContactMode string    `json:"contactMode,omitempty"`
	EventMode   string    `json:"eventMode,omitempty"`
	Warnings    []string  `json:"warnings,omitempty"`
	SyncedAt    time.Time `json:"syncedAt"`
}

// Hit 表示本地搜索结果。
type Hit struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle,omitempty"`
	Snippet   string    `json:"snippet,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// IndexPath 返回本地索引文件路径。
func IndexPath() (string, error) {
	dir, err := store.EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.json"), nil
}

// Load 读取本地索引。
func Load() (*Snapshot, error) {
	path, err := IndexPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Snapshot{Version: indexVersion, Messages: []graph.Message{}, Contacts: []graph.Contact{}, Events: []graph.CalendarEvent{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取本地索引失败: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("解析本地索引失败: %w", err)
	}
	if snapshot.Version == 0 {
		snapshot.Version = indexVersion
	}
	if snapshot.Messages == nil {
		snapshot.Messages = []graph.Message{}
	}
	if snapshot.Contacts == nil {
		snapshot.Contacts = []graph.Contact{}
	}
	if snapshot.Events == nil {
		snapshot.Events = []graph.CalendarEvent{}
	}
	return &snapshot, nil
}

// Save 保存本地索引。
func Save(snapshot *Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("索引快照不能为空")
	}
	snapshot.Version = indexVersion
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化本地索引失败: %w", err)
	}
	path, err := IndexPath()
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, data, 0o600)
}

// Sync 执行邮件、联系人和日历同步。
func Sync(ctx context.Context, accessToken string, snapshot *Snapshot, options SyncOptions) (*SyncReport, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("索引快照不能为空")
	}
	if options.Max <= 0 {
		options.Max = 1000
	}
	report := &SyncReport{}
	if options.Messages {
		mode, warning := syncMessages(ctx, accessToken, snapshot, options.Max)
		report.MessageMode = mode
		if warning != "" {
			report.Warnings = append(report.Warnings, warning)
		}
		report.Messages = len(snapshot.Messages)
	}
	if options.Contacts {
		mode, warning := syncContacts(ctx, accessToken, snapshot, options.Max)
		report.ContactMode = mode
		if warning != "" {
			report.Warnings = append(report.Warnings, warning)
		}
		report.Contacts = len(snapshot.Contacts)
	}
	if options.Events {
		mode, warning := syncEvents(ctx, accessToken, snapshot, options.Max)
		report.EventMode = mode
		if warning != "" {
			report.Warnings = append(report.Warnings, warning)
		}
		report.Events = len(snapshot.Events)
	}
	report.SyncedAt = time.Now().UTC()
	snapshot.SyncedAt = report.SyncedAt
	if err := Save(snapshot); err != nil {
		return report, err
	}
	return report, nil
}

func syncMessages(ctx context.Context, accessToken string, snapshot *Snapshot, max int) (string, string) {
	result, err := graph.DeltaMessages(ctx, accessToken, snapshot.MessageDelta, max)
	if err == nil {
		snapshot.Messages = upsertMessages(snapshot.Messages, result.Items)
		snapshot.Messages = removeMessages(snapshot.Messages, result.Removed)
		snapshot.MessageDelta = result.DeltaLink
		return "delta", ""
	}
	items, fullErr := graph.ListAllMessagesAll(ctx, accessToken, max)
	if fullErr != nil {
		return "failed", fmt.Sprintf("邮件同步失败: delta=%v; full=%v", err, fullErr)
	}
	snapshot.Messages = items
	snapshot.MessageDelta = ""
	return "full-fallback", fmt.Sprintf("邮件 delta 不可用，已回退全量: %v", err)
}

func syncContacts(ctx context.Context, accessToken string, snapshot *Snapshot, max int) (string, string) {
	result, err := graph.DeltaContacts(ctx, accessToken, snapshot.ContactDelta, max)
	if err == nil {
		snapshot.Contacts = upsertContacts(snapshot.Contacts, result.Items)
		snapshot.Contacts = removeContacts(snapshot.Contacts, result.Removed)
		snapshot.ContactDelta = result.DeltaLink
		return "delta", ""
	}
	items, fullErr := graph.ListContactsInFolderAll(ctx, accessToken, "", max)
	if fullErr != nil {
		return "failed", fmt.Sprintf("联系人同步失败: delta=%v; full=%v", err, fullErr)
	}
	snapshot.Contacts = items
	snapshot.ContactDelta = ""
	return "full-fallback", fmt.Sprintf("联系人 delta 不可用，已回退全量: %v", err)
}

func syncEvents(ctx context.Context, accessToken string, snapshot *Snapshot, max int) (string, string) {
	result, err := graph.DeltaEvents(ctx, accessToken, snapshot.EventDelta, max)
	if err == nil {
		snapshot.Events = upsertEvents(snapshot.Events, result.Items)
		snapshot.Events = removeEvents(snapshot.Events, result.Removed)
		snapshot.EventDelta = result.DeltaLink
		return "delta", ""
	}
	items, fullErr := graph.ListCalendarEventsAll(ctx, accessToken, "", max)
	if fullErr != nil {
		return "failed", fmt.Sprintf("日历同步失败: delta=%v; full=%v", err, fullErr)
	}
	snapshot.Events = items
	snapshot.EventDelta = ""
	return "full-fallback", fmt.Sprintf("日历 delta 不可用，已回退全量: %v", err)
}

func upsertMessages(items, updates []graph.Message) []graph.Message {
	index := map[string]int{}
	for i, item := range items {
		index[item.ID] = i
	}
	for _, update := range updates {
		if i, ok := index[update.ID]; ok {
			items[i] = update
		} else {
			index[update.ID] = len(items)
			items = append(items, update)
		}
	}
	return items
}

func upsertContacts(items, updates []graph.Contact) []graph.Contact {
	index := map[string]int{}
	for i, item := range items {
		index[item.ID] = i
	}
	for _, update := range updates {
		if i, ok := index[update.ID]; ok {
			items[i] = update
		} else {
			index[update.ID] = len(items)
			items = append(items, update)
		}
	}
	return items
}

func upsertEvents(items, updates []graph.CalendarEvent) []graph.CalendarEvent {
	index := map[string]int{}
	for i, item := range items {
		index[item.ID] = i
	}
	for _, update := range updates {
		if i, ok := index[update.ID]; ok {
			items[i] = update
		} else {
			index[update.ID] = len(items)
			items = append(items, update)
		}
	}
	return items
}

func removeMessages(items []graph.Message, removed []string) []graph.Message {
	set := stringSet(removed)
	result := items[:0]
	for _, item := range items {
		if !set[item.ID] {
			result = append(result, item)
		}
	}
	return result
}

func removeContacts(items []graph.Contact, removed []string) []graph.Contact {
	set := stringSet(removed)
	result := items[:0]
	for _, item := range items {
		if !set[item.ID] {
			result = append(result, item)
		}
	}
	return result
}

func removeEvents(items []graph.CalendarEvent, removed []string) []graph.CalendarEvent {
	set := stringSet(removed)
	result := items[:0]
	for _, item := range items {
		if !set[item.ID] {
			result = append(result, item)
		}
	}
	return result
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

// Search 在本地索引中搜索。
func Search(snapshot *Snapshot, query, kind string, limit int) []Hit {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	hits := make([]Hit, 0)
	if kind == "" || kind == "all" || kind == "mail" {
		for _, message := range snapshot.Messages {
			haystack := strings.ToLower(message.Subject + "\n" + message.BodyPreview + "\n" + message.From.EmailAddress.Address)
			if strings.Contains(haystack, query) {
				hits = append(hits, Hit{Type: "mail", ID: message.ID, Title: message.Subject, Subtitle: message.From.EmailAddress.Address, Snippet: message.BodyPreview, Timestamp: message.ReceivedDateTime})
			}
		}
	}
	if kind == "" || kind == "all" || kind == "contact" {
		for _, contact := range snapshot.Contacts {
			emails := make([]string, 0, len(contact.EmailAddresses))
			for _, email := range contact.EmailAddresses {
				emails = append(emails, email.Address)
			}
			haystack := strings.ToLower(contact.DisplayName + "\n" + strings.Join(emails, "\n") + "\n" + contact.MobilePhone + "\n" + contact.CompanyName)
			if strings.Contains(haystack, query) {
				hits = append(hits, Hit{Type: "contact", ID: contact.ID, Title: contact.DisplayName, Subtitle: strings.Join(emails, ", "), Snippet: contact.CompanyName})
			}
		}
	}
	if kind == "" || kind == "all" || kind == "event" {
		for _, event := range snapshot.Events {
			haystack := strings.ToLower(event.Subject + "\n" + event.BodyPreview + "\n" + event.Location.DisplayName)
			if strings.Contains(haystack, query) {
				timestamp, _ := time.Parse(time.RFC3339, event.Start.DateTime)
				hits = append(hits, Hit{Type: "event", ID: event.ID, Title: event.Subject, Subtitle: event.Location.DisplayName, Snippet: event.BodyPreview, Timestamp: timestamp})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		return hits[i].Timestamp.After(hits[j].Timestamp)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
