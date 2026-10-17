package graph

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// MailAttachmentInput 表示待发送的本地文件附件。
type MailAttachmentInput struct {
	Name         string
	ContentType  string
	ContentBytes []byte
}

// LoadMailAttachments 从本地文件路径加载附件。
// 当前使用 Graph 的简单附件方式，所有附件总大小限制为 3 MiB。
func LoadMailAttachments(paths []string) ([]MailAttachmentInput, error) {
	const maxTotalSize = 3 * 1024 * 1024
	attachments := make([]MailAttachmentInput, 0, len(paths))
	total := 0
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取附件失败 (%s): %w", path, err)
		}
		total += len(data)
		if total > maxTotalSize {
			return nil, fmt.Errorf("附件总大小超过 3 MiB，暂不支持通过简单附件发送")
		}
		name := filepath.Base(path)
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		attachments = append(attachments, MailAttachmentInput{
			Name:         name,
			ContentType:  contentType,
			ContentBytes: data,
		})
	}
	return attachments, nil
}
