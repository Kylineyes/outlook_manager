package eml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

const simpleAttachmentLimit = 3 * 1024 * 1024

// DraftInput 表示从 EML 转换出的草稿输入和临时文件。
type DraftInput struct {
	Input   graph.SendMailInput
	tempDir string
}

// ToDraftInput 把解析后的 EML 转换为 Graph 草稿输入。
// 超过 3 MiB 的附件会写入临时文件，由草稿上传会话处理。
func ToDraftInput(message *Message) (*DraftInput, error) {
	if message == nil {
		return nil, fmt.Errorf("EML 邮件不能为空")
	}
	if len(message.To) == 0 && len(message.Cc) == 0 && len(message.Bcc) == 0 {
		return nil, fmt.Errorf("EML 中没有收件人")
	}
	body := message.HTMLBody
	contentType := "HTML"
	if strings.TrimSpace(body) == "" {
		body = message.TextBody
		contentType = "Text"
	}
	result := &DraftInput{
		Input: graph.SendMailInput{
			To:          message.To,
			Cc:          message.Cc,
			Bcc:         message.Bcc,
			Subject:     message.Subject,
			Body:        body,
			ContentType: contentType,
		},
	}
	if len(message.Attachments) == 0 {
		return result, nil
	}
	tempDir, err := os.MkdirTemp("", "outlook-eml-*")
	if err != nil {
		return nil, fmt.Errorf("创建 EML 附件临时目录失败: %w", err)
	}
	result.tempDir = tempDir
	for index, attachment := range message.Attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = fmt.Sprintf("attachment-%d", index+1)
		}
		name = filepath.Base(name)
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		input := graph.MailAttachmentInput{
			Name:        name,
			ContentType: contentType,
			Size:        int64(len(attachment.Content)),
		}
		if len(attachment.Content) <= simpleAttachmentLimit {
			input.ContentBytes = attachment.Content
		} else {
			path := filepath.Join(tempDir, fmt.Sprintf("%d-%s", index+1, name))
			if err := os.WriteFile(path, attachment.Content, 0o600); err != nil {
				result.Cleanup()
				return nil, fmt.Errorf("写入 EML 临时附件失败: %w", err)
			}
			input.Path = path
		}
		result.Input.Attachments = append(result.Input.Attachments, input)
	}
	return result, nil
}

// Cleanup 删除 EML 导入过程中创建的临时文件。
func (input *DraftInput) Cleanup() {
	if input == nil || input.tempDir == "" {
		return
	}
	_ = os.RemoveAll(input.tempDir)
	input.tempDir = ""
}
