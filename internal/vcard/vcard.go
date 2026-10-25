// Package vcard 负责联系人 vCard 3.0 的导入和导出。
package vcard

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

// ExportFile 把联系人导出为 vCard 3.0 文件。
func ExportFile(path string, contacts []graph.Contact) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("创建 vCard 文件失败: %w", err)
	}
	defer file.Close()
	return WriteContacts(file, contacts)
}

// WriteContacts 把联系人写入 vCard 3.0。
func WriteContacts(w io.Writer, contacts []graph.Contact) error {
	writer := bufio.NewWriter(w)
	for _, contact := range contacts {
		fmt.Fprintln(writer, "BEGIN:VCARD")
		fmt.Fprintln(writer, "VERSION:3.0")
		fmt.Fprintf(writer, "FN:%s\n", escape(contact.DisplayName))
		fmt.Fprintf(writer, "N:%s;%s;;;\n", escape(contact.Surname), escape(contact.GivenName))
		for _, email := range contact.EmailAddresses {
			if strings.TrimSpace(email.Address) != "" {
				fmt.Fprintf(writer, "EMAIL;TYPE=INTERNET:%s\n", escape(email.Address))
			}
		}
		if contact.MobilePhone != "" {
			fmt.Fprintf(writer, "TEL;TYPE=CELL:%s\n", escape(contact.MobilePhone))
		}
		for _, phone := range contact.BusinessPhones {
			if strings.TrimSpace(phone) != "" {
				fmt.Fprintf(writer, "TEL;TYPE=WORK:%s\n", escape(phone))
			}
		}
		if contact.CompanyName != "" {
			fmt.Fprintf(writer, "ORG:%s\n", escape(contact.CompanyName))
		}
		if contact.JobTitle != "" {
			fmt.Fprintf(writer, "TITLE:%s\n", escape(contact.JobTitle))
		}
		if contact.NickName != "" {
			fmt.Fprintf(writer, "NICKNAME:%s\n", escape(contact.NickName))
		}
		if contact.Birthday != "" {
			fmt.Fprintf(writer, "BDAY:%s\n", escape(contact.Birthday))
		}
		if contact.PersonalNotes != "" {
			fmt.Fprintf(writer, "NOTE:%s\n", escape(contact.PersonalNotes))
		}
		if contact.BusinessHomePage != "" {
			fmt.Fprintf(writer, "URL:%s\n", escape(contact.BusinessHomePage))
		}
		writeAddress := func(kind string, address *graph.PhysicalAddress) {
			if address == nil {
				return
			}
			parts := []string{"", "", address.Street, address.City, address.State, address.PostalCode, address.CountryOrRegion}
			if strings.TrimSpace(strings.Join(parts, "")) == "" {
				return
			}
			for i := range parts {
				parts[i] = escape(parts[i])
			}
			fmt.Fprintf(writer, "ADR;TYPE=%s:%s\n", kind, strings.Join(parts, ";"))
		}
		writeAddress("HOME", contact.HomeAddress)
		writeAddress("WORK", contact.BusinessAddress)
		writeAddress("OTHER", contact.OtherAddress)
		fmt.Fprintln(writer, "END:VCARD")
	}
	return writer.Flush()
}

// ReadFile 从 vCard 文件读取联系人。
func ReadFile(path string) ([]graph.ContactInput, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 vCard 文件失败: %w", err)
	}
	defer file.Close()
	return ReadContacts(file)
}

// ReadContacts 从 vCard 3.0 文本读取联系人。
func ReadContacts(r io.Reader) ([]graph.ContactInput, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	lines := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += strings.TrimLeft(line, " \t")
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 vCard 失败: %w", err)
	}
	contacts := make([]graph.ContactInput, 0)
	var current *graph.ContactInput
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if upper == "BEGIN:VCARD" {
			current = &graph.ContactInput{}
			continue
		}
		if upper == "END:VCARD" {
			if current != nil {
				if current.DisplayName == "" {
					current.DisplayName = strings.TrimSpace(current.GivenName + " " + current.Surname)
				}
				if current.DisplayName != "" {
					contacts = append(contacts, *current)
				}
			}
			current = nil
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := splitProperty(line)
		if !ok {
			continue
		}
		key = strings.ToUpper(key)
		value = unescape(value)
		switch key {
		case "FN":
			current.DisplayName = value
		case "N":
			parts := strings.Split(value, ";")
			if len(parts) > 0 {
				current.Surname = strings.TrimSpace(parts[0])
			}
			if len(parts) > 1 {
				current.GivenName = strings.TrimSpace(parts[1])
			}
		case "EMAIL":
			if value != "" {
				current.EmailAddresses = append(current.EmailAddresses, graph.ContactEmailAddress{Address: value})
			}
		case "TEL":
			if strings.Contains(upper, "TYPE=WORK") {
				current.BusinessPhones = append(current.BusinessPhones, value)
			} else if value != "" {
				current.MobilePhone = value
			}
		case "ORG":
			current.CompanyName = strings.TrimSpace(strings.Split(value, ";")[0])
		case "TITLE":
			current.JobTitle = value
		case "NICKNAME":
			current.NickName = value
		case "BDAY":
			current.Birthday = normalizeBirthday(value)
		case "NOTE":
			current.PersonalNotes = value
		case "URL":
			current.BusinessHomePage = value
		case "ADR":
			address := parseAddress(value)
			switch {
			case strings.Contains(upper, "TYPE=HOME"):
				current.HomeAddress = address
			case strings.Contains(upper, "TYPE=WORK"):
				current.BusinessAddress = address
			case strings.Contains(upper, "TYPE=OTHER"):
				current.OtherAddress = address
			}
		}
	}
	if len(contacts) == 0 {
		return nil, fmt.Errorf("vCard 文件中没有有效联系人")
	}
	return contacts, nil
}

func splitProperty(line string) (string, string, bool) {
	index := strings.Index(line, ":")
	if index < 0 {
		return "", "", false
	}
	left := line[:index]
	if semicolon := strings.Index(left, ";"); semicolon >= 0 {
		left = left[:semicolon]
	}
	return left, line[index+1:], true
}

func parseAddress(value string) *graph.PhysicalAddress {
	parts := strings.Split(value, ";")
	for len(parts) < 7 {
		parts = append(parts, "")
	}
	address := &graph.PhysicalAddress{
		Street:          strings.TrimSpace(parts[2]),
		City:            strings.TrimSpace(parts[3]),
		State:           strings.TrimSpace(parts[4]),
		PostalCode:      strings.TrimSpace(parts[5]),
		CountryOrRegion: strings.TrimSpace(parts[6]),
	}
	if address.Street == "" && address.City == "" && address.State == "" && address.PostalCode == "" && address.CountryOrRegion == "" {
		return nil
	}
	return address
}

func normalizeBirthday(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 10 {
		if parsed, err := time.Parse("2006-01-02", value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return value
}

func escape(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
	)
	return replacer.Replace(value)
}

func unescape(value string) string {
	replacer := strings.NewReplacer(
		"\\n", "\n",
		"\\N", "\n",
		"\\,", ",",
		"\\;", ";",
		"\\\\", "\\",
	)
	return replacer.Replace(value)
}
