package eml

import (
	"strings"
	"testing"
)

func TestParseMultipartMessage(t *testing.T) {
	raw := "From: Sender <sender@example.com>\r\n" +
		"To: Receiver <receiver@example.com>\r\n" +
		"Cc: copy@example.com\r\n" +
		"Subject: =?UTF-8?B?5rWL6K+V6YKu5Lu2?=\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0800\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=outer\r\n" +
		"\r\n" +
		"--outer\r\n" +
		"Content-Type: multipart/alternative; boundary=inner\r\n" +
		"\r\n" +
		"--inner\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"hello=20world\r\n" +
		"--inner\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>hello world</p>\r\n" +
		"--inner--\r\n" +
		"--outer\r\n" +
		"Content-Type: text/plain; name=test.txt\r\n" +
		"Content-Disposition: attachment; filename=test.txt\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"aGVsbG8gYXR0YWNobWVudA==\r\n" +
		"--outer--\r\n"
	message, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Subject != "测试邮件" {
		t.Fatalf("subject = %q", message.Subject)
	}
	if len(message.To) != 1 || message.To[0] != "receiver@example.com" {
		t.Fatalf("to = %#v", message.To)
	}
	if message.TextBody != "hello world" || message.HTMLBody != "<p>hello world</p>" {
		t.Fatalf("body = %q / %q", message.TextBody, message.HTMLBody)
	}
	if len(message.Attachments) != 1 || string(message.Attachments[0].Content) != "hello attachment" {
		t.Fatalf("attachments = %#v", message.Attachments)
	}
}

func TestParseISO2022JPSubject(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"To: receiver@example.com\r\n" +
		"Subject: =?ISO-2022-JP?B?GyRCJUYlOSVIGyhC?=\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=iso-2022-jp\r\n" +
		"Content-Transfer-Encoding: 7bit\r\n" +
		"\r\n" +
		"body\r\n"
	message, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Subject != "テスト" {
		t.Fatalf("subject = %q", message.Subject)
	}
}
