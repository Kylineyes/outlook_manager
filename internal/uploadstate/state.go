// Package uploadstate 保存大附件上传的断点状态。
package uploadstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/store"
)

// AttachmentState 表示单个附件的上传会话状态。
type AttachmentState struct {
	Key       string    `json:"key"`
	DraftID   string    `json:"draftId"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	Offset    int64     `json:"offset"`
	UploadURL string    `json:"uploadUrl"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SendState 表示一次带大附件发送的草稿状态。
type SendState struct {
	Key         string    `json:"key"`
	DraftID     string    `json:"draftId"`
	UploadsDone bool      `json:"uploadsDone"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Dir 返回上传状态目录。
func Dir() (string, error) {
	base, err := store.EnsureBaseDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "uploads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建上传状态目录失败: %w", err)
	}
	return dir, nil
}

func statePath(prefix, key string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, prefix+"-"+key+".json"), nil
}

// SaveAttachment 保存附件上传状态。
func SaveAttachment(state AttachmentState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化附件上传状态失败: %w", err)
	}
	path, err := statePath("attachment", state.Key)
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, data, 0o600)
}

// LoadAttachment 读取附件上传状态。
func LoadAttachment(key string) (*AttachmentState, error) {
	path, err := statePath("attachment", key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取附件上传状态失败: %w", err)
	}
	var state AttachmentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析附件上传状态失败: %w", err)
	}
	return &state, nil
}

// DeleteAttachment 删除附件上传状态。
func DeleteAttachment(key string) error {
	path, err := statePath("attachment", key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// SaveSend 保存发送草稿状态。
func SaveSend(state SendState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化发送状态失败: %w", err)
	}
	path, err := statePath("send", state.Key)
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, data, 0o600)
}

// LoadSend 读取发送草稿状态。
func LoadSend(key string) (*SendState, error) {
	path, err := statePath("send", key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取发送状态失败: %w", err)
	}
	var state SendState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析发送状态失败: %w", err)
	}
	return &state, nil
}

// DeleteSend 删除发送草稿状态。
func DeleteSend(key string) error {
	path, err := statePath("send", key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
