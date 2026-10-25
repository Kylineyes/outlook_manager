// Package contactcsv 负责联系人 CSV 的导入和导出。
package contactcsv

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

var headers = []string{
	"displayName", "givenName", "surname", "emails", "companyName", "jobTitle",
	"mobilePhone", "businessPhones", "nickName", "birthday", "personalNotes",
	"department", "officeLocation", "profession", "businessHomePage",
	"homeAddress", "businessAddress", "otherAddress",
}

// ExportFile 把联系人导出为 UTF-8 CSV 文件。
func ExportFile(path string, contacts []graph.Contact) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("创建联系人 CSV 失败: %w", err)
	}
	defer file.Close()
	return WriteContacts(file, contacts)
}

// WriteContacts 把联系人写入 CSV。
func WriteContacts(w io.Writer, contacts []graph.Contact) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(headers); err != nil {
		return err
	}
	for _, contact := range contacts {
		row := []string{
			contact.DisplayName,
			contact.GivenName,
			contact.Surname,
			joinEmails(contact.EmailAddresses),
			contact.CompanyName,
			contact.JobTitle,
			contact.MobilePhone,
			strings.Join(contact.BusinessPhones, ";"),
			contact.NickName,
			contact.Birthday,
			contact.PersonalNotes,
			contact.Department,
			contact.OfficeLocation,
			contact.Profession,
			contact.BusinessHomePage,
			formatAddress(contact.HomeAddress),
			formatAddress(contact.BusinessAddress),
			formatAddress(contact.OtherAddress),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// ReadFile 从 UTF-8 CSV 文件读取联系人。
func ReadFile(path string) ([]graph.ContactInput, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开联系人 CSV 失败: %w", err)
	}
	defer file.Close()
	return ReadContacts(file)
}

// ReadContacts 从 CSV 读取联系人。
func ReadContacts(r io.Reader) ([]graph.ContactInput, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析联系人 CSV 失败: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("联系人 CSV 为空")
	}
	index := map[string]int{}
	for i, name := range records[0] {
		index[strings.TrimPrefix(strings.TrimSpace(name), "\ufeff")] = i
	}
	for _, required := range []string{"displayName"} {
		if _, ok := index[required]; !ok {
			return nil, fmt.Errorf("联系人 CSV 缺少列: %s", required)
		}
	}
	contacts := make([]graph.ContactInput, 0, len(records)-1)
	for rowNumber, row := range records[1:] {
		if isBlankRow(row) {
			continue
		}
		input := graph.ContactInput{
			DisplayName:      value(row, index, "displayName"),
			GivenName:        value(row, index, "givenName"),
			Surname:          value(row, index, "surname"),
			CompanyName:      value(row, index, "companyName"),
			JobTitle:         value(row, index, "jobTitle"),
			MobilePhone:      value(row, index, "mobilePhone"),
			NickName:         value(row, index, "nickName"),
			Birthday:         value(row, index, "birthday"),
			PersonalNotes:    value(row, index, "personalNotes"),
			Department:       value(row, index, "department"),
			OfficeLocation:   value(row, index, "officeLocation"),
			Profession:       value(row, index, "profession"),
			BusinessHomePage: value(row, index, "businessHomePage"),
			HomeAddress:      parseAddress(value(row, index, "homeAddress")),
			BusinessAddress:  parseAddress(value(row, index, "businessAddress")),
			OtherAddress:     parseAddress(value(row, index, "otherAddress")),
		}
		for _, email := range splitMulti(value(row, index, "emails")) {
			input.EmailAddresses = append(input.EmailAddresses, graph.ContactEmailAddress{Address: email})
		}
		input.BusinessPhones = splitMulti(value(row, index, "businessPhones"))
		if strings.TrimSpace(input.DisplayName) == "" {
			return nil, fmt.Errorf("联系人 CSV 第 %d 行缺少 displayName", rowNumber+2)
		}
		contacts = append(contacts, input)
	}
	return contacts, nil
}

func joinEmails(emails []graph.ContactEmailAddress) string {
	values := make([]string, 0, len(emails))
	for _, email := range emails {
		if strings.TrimSpace(email.Address) != "" {
			values = append(values, strings.TrimSpace(email.Address))
		}
	}
	return strings.Join(values, ";")
}

func formatAddress(address *graph.PhysicalAddress) string {
	if address == nil {
		return ""
	}
	parts := []string{address.Street, address.City, address.State, address.PostalCode, address.CountryOrRegion}
	empty := true
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			empty = false
			break
		}
	}
	if empty {
		return ""
	}
	return strings.Join(parts, "|")
}

func parseAddress(value string) *graph.PhysicalAddress {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "|")
	for len(parts) < 5 {
		parts = append(parts, "")
	}
	address := &graph.PhysicalAddress{
		Street:          strings.TrimSpace(parts[0]),
		City:            strings.TrimSpace(parts[1]),
		State:           strings.TrimSpace(parts[2]),
		PostalCode:      strings.TrimSpace(parts[3]),
		CountryOrRegion: strings.TrimSpace(parts[4]),
	}
	if address.Street == "" && address.City == "" && address.State == "" && address.PostalCode == "" && address.CountryOrRegion == "" {
		return nil
	}
	return address
}

func splitMulti(value string) []string {
	value = strings.ReplaceAll(value, "；", ";")
	parts := strings.Split(value, ";")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func value(row []string, index map[string]int, name string) string {
	position, ok := index[name]
	if !ok || position >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[position])
}

func isBlankRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
