package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MailView 表示一个保存的邮件查询视图。
type MailView struct {
	Name          string `json:"name"`
	Folder        string `json:"folder,omitempty"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	Subject       string `json:"subject,omitempty"`
	Body          string `json:"body,omitempty"`
	Search        string `json:"search,omitempty"`
	Importance    string `json:"importance,omitempty"`
	Category      string `json:"category,omitempty"`
	After         string `json:"after,omitempty"`
	Before        string `json:"before,omitempty"`
	UnreadOnly    bool   `json:"unreadOnly,omitempty"`
	HasAttachment bool   `json:"hasAttachment,omitempty"`
	Top           int    `json:"top,omitempty"`
}

// MailViewsFile 表示 views.json 的结构。
type MailViewsFile struct {
	Version int        `json:"version"`
	Views   []MailView `json:"views"`
}

// MailViewsPath 返回 views.json 的完整路径。
func MailViewsPath() (string, error) {
	dir, err := EnsureBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "views.json"), nil
}

// LoadMailViews 读取保存的邮件视图。
func LoadMailViews() (MailViewsFile, error) {
	path, err := MailViewsPath()
	if err != nil {
		return MailViewsFile{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return MailViewsFile{Version: 1, Views: []MailView{}}, nil
	}
	if err != nil {
		return MailViewsFile{}, fmt.Errorf("读取邮件视图失败: %w", err)
	}
	var file MailViewsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return MailViewsFile{}, fmt.Errorf("解析邮件视图失败: %w", err)
	}
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Views == nil {
		file.Views = []MailView{}
	}
	return file, nil
}

// SaveMailViews 保存邮件视图。
func SaveMailViews(file MailViewsFile) error {
	if file.Version == 0 {
		file.Version = 1
	}
	if file.Views == nil {
		file.Views = []MailView{}
	}
	sort.SliceStable(file.Views, func(i, j int) bool {
		return file.Views[i].Name < file.Views[j].Name
	})
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化邮件视图失败: %w", err)
	}
	path, err := MailViewsPath()
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data, 0o600)
}

// UpsertMailView 新增或更新一个邮件视图。
func UpsertMailView(view MailView) (MailViewsFile, error) {
	file, err := LoadMailViews()
	if err != nil {
		return file, err
	}
	updated := false
	for i := range file.Views {
		if strings.EqualFold(file.Views[i].Name, view.Name) {
			file.Views[i] = view
			updated = true
			break
		}
	}
	if !updated {
		file.Views = append(file.Views, view)
	}
	if err := SaveMailViews(file); err != nil {
		return file, err
	}
	return file, nil
}

// DeleteMailView 删除一个邮件视图。
func DeleteMailView(name string) (MailViewsFile, error) {
	file, err := LoadMailViews()
	if err != nil {
		return file, err
	}
	views := make([]MailView, 0, len(file.Views))
	found := false
	for _, view := range file.Views {
		if strings.EqualFold(view.Name, name) {
			found = true
			continue
		}
		views = append(views, view)
	}
	if !found {
		return file, fmt.Errorf("找不到邮件视图: %s", name)
	}
	file.Views = views
	if err := SaveMailViews(file); err != nil {
		return file, err
	}
	return file, nil
}
