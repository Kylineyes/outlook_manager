// Package eml 负责解析 EML/MIME 邮件文件。
package eml

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"strings"
	"time"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

const maxAttachmentSize = 150 * 1024 * 1024

// Attachment 表示 EML 中的附件。
type Attachment struct {
	Name        string
	ContentType string
	Content     []byte
}

// Message 表示从 EML 解析出的邮件内容。
type Message struct {
	Subject     string
	To          []string
	Cc          []string
	Bcc         []string
	TextBody    string
	HTMLBody    string
	Date        time.Time
	Attachments []Attachment
}

// ParseFile 解析本地 EML 文件。
func ParseFile(path string) (*Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 EML 文件失败: %w", err)
	}
	defer file.Close()
	return Parse(file)
}

// Parse 从 reader 解析 EML/MIME 邮件。
func Parse(r io.Reader) (*Message, error) {
	parsed, err := mail.ReadMessage(r)
	if err != nil {
		return nil, fmt.Errorf("解析 EML 邮件头失败: %w", err)
	}
	message := &Message{
		Subject: decodeHeader(parsed.Header.Get("Subject")),
		To:      parseAddresses(parsed.Header.Get("To")),
		Cc:      parseAddresses(parsed.Header.Get("Cc")),
		Bcc:     parseAddresses(parsed.Header.Get("Bcc")),
	}
	if dateValue := parsed.Header.Get("Date"); dateValue != "" {
		if date, err := mail.ParseDate(dateValue); err == nil {
			message.Date = date
		}
	}
	if err := walkPart(textproto.MIMEHeader(parsed.Header), parsed.Body, message); err != nil {
		return nil, err
	}
	if message.TextBody == "" && message.HTMLBody == "" && len(message.Attachments) == 0 {
		return nil, fmt.Errorf("EML 中没有可导入的正文或附件")
	}
	return message, nil
}

func walkPart(header textproto.MIMEHeader, reader io.Reader, message *Message) error {
	contentType := header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("解析 MIME Content-Type 失败: %w", err)
	}
	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return fmt.Errorf("MIME multipart 缺少 boundary")
		}
		multipartReader := multipart.NewReader(reader, boundary)
		for {
			part, err := multipartReader.NextPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("读取 MIME multipart 失败: %w", err)
			}
			if err := walkPart(part.Header, part, message); err != nil {
				_ = part.Close()
				return err
			}
			_ = part.Close()
		}
	}
	disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := dispositionParams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	filename = decodeHeader(filename)
	isAttachment := strings.EqualFold(disposition, "attachment") || filename != ""
	data, err := readDecoded(reader, header.Get("Content-Transfer-Encoding"))
	if err != nil {
		return err
	}
	if isAttachment {
		if filename == "" {
			filename = fmt.Sprintf("attachment-%d", len(message.Attachments)+1)
		}
		if len(data) > maxAttachmentSize {
			return fmt.Errorf("EML 附件 %s 超过 150 MiB 上限", filename)
		}
		if strings.TrimSpace(mediaType) == "" {
			mediaType = "application/octet-stream"
		}
		message.Attachments = append(message.Attachments, Attachment{Name: filename, ContentType: mediaType, Content: data})
		return nil
	}
	switch strings.ToLower(mediaType) {
	case "text/plain":
		if message.TextBody == "" {
			message.TextBody = decodeCharset(data, params["charset"])
		}
	case "text/html":
		if message.HTMLBody == "" {
			message.HTMLBody = decodeCharset(data, params["charset"])
		}
	}
	return nil
}

func readDecoded(reader io.Reader, transferEncoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(transferEncoding)) {
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, reader))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(reader))
	default:
		return io.ReadAll(reader)
	}
}

func decodeHeader(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	decoder := &mime.WordDecoder{CharsetReader: charsetReader}
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	encoding, err := htmlindex.Get(charset)
	if err != nil {
		return nil, err
	}
	return transform.NewReader(input, encoding.NewDecoder()), nil
}

func decodeCharset(data []byte, charset string) string {
	if strings.TrimSpace(charset) == "" {
		return string(data)
	}
	encoding, err := htmlindex.Get(charset)
	if err != nil {
		return string(data)
	}
	decoded, err := io.ReadAll(transform.NewReader(bytes.NewReader(data), encoding.NewDecoder()))
	if err != nil {
		return string(data)
	}
	return string(decoded)
}

func parseAddresses(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	addresses, err := mail.ParseAddressList(value)
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, address.Address)
	}
	return result
}
