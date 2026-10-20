package graph

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

const (
	// simpleAttachmentLimit 是 Graph 单次 POST 附件的建议上限。
	simpleAttachmentLimit = 3 * 1024 * 1024
	// maxMailAttachmentSize 是 Graph 邮件附件的单文件上限。
	maxMailAttachmentSize = 150 * 1024 * 1024
)

// MailAttachmentInput 表示待发送的本地文件附件。
// 小文件直接放在 ContentBytes 中；大文件保留 Path，通过上传会话发送。
type MailAttachmentInput struct {
	Name         string
	ContentType  string
	ContentBytes []byte
	Path         string
	Size         int64
}

// LoadMailAttachments 从本地文件路径加载附件。
// 小于等于 3 MiB 的文件直接读入内存；更大的文件保留路径，后续使用 Graph 上传会话。
func LoadMailAttachments(paths []string) ([]MailAttachmentInput, error) {
	attachments := make([]MailAttachmentInput, 0, len(paths))
	var totalSize int64
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("读取附件信息失败 (%s): %w", path, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("附件路径是目录 (%s)", path)
		}
		if info.Size() > maxMailAttachmentSize {
			return nil, fmt.Errorf("附件 %s 超过 150 MiB 上限", filepath.Base(path))
		}
		totalSize += info.Size()
		if totalSize > maxMailAttachmentSize {
			return nil, fmt.Errorf("附件总大小超过 150 MiB 上限")
		}
		name := filepath.Base(path)
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		attachment := MailAttachmentInput{
			Name:        name,
			ContentType: contentType,
			Path:        path,
			Size:        info.Size(),
		}
		if info.Size() <= simpleAttachmentLimit {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("读取附件失败 (%s): %w", path, err)
			}
			attachment.ContentBytes = data
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

// hasLargeAttachments 判断是否需要通过草稿和上传会话流程发送附件。
// 单个文件超过 3 MiB，或附件总大小超过 3 MiB 时都需要走草稿流程。
func hasLargeAttachments(attachments []MailAttachmentInput) bool {
	var totalSize int64
	for _, attachment := range attachments {
		totalSize += attachment.Size
		if attachment.Path != "" && attachment.Size > simpleAttachmentLimit {
			return true
		}
	}
	return totalSize > simpleAttachmentLimit
}
