package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Kylineyes/outlook-manager/internal/cliconfig"
	"github.com/Kylineyes/outlook-manager/internal/cliui"
	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func effectiveOutputFormat(option string, jsonFlag bool) (string, error) {
	if jsonFlag {
		return "json", nil
	}
	value := strings.ToLower(strings.TrimSpace(option))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(cliconfig.Load().Output))
	}
	if value == "" {
		value = "text"
	}
	switch value {
	case "text", "table", "json", "yaml", "jsonl":
		return value, nil
	default:
		return "", fmt.Errorf("不支持的输出格式: %s", value)
	}
}

func printMailTable(messages []graph.Message, username string) error {
	rows := make([][]string, 0, len(messages))
	for _, message := range messages {
		readState := "未读"
		if message.IsRead {
			readState = "已读"
		}
		sender := message.From.EmailAddress.Address
		if sender == "" {
			sender = message.From.EmailAddress.Name
		}
		attachment := ""
		if message.HasAttachments {
			attachment = "有"
		}
		rows = append(rows, []string{
			message.ReceivedDateTime.Local().Format("2006-01-02 15:04"),
			readState,
			sender,
			message.Subject,
			attachment,
		})
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", username)
	return cliui.PrintTable(os.Stdout, []string{"时间", "状态", "发件人", "主题", "附件"}, rows)
}

func printCalendarTable(events []graph.CalendarEvent, username string) error {
	rows := make([][]string, 0, len(events))
	for _, event := range events {
		rows = append(rows, []string{
			event.Start.DateTime,
			event.End.DateTime,
			event.Subject,
			event.Location.DisplayName,
			event.ShowAs,
		})
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", username)
	return cliui.PrintTable(os.Stdout, []string{"开始", "结束", "主题", "地点", "状态"}, rows)
}

func printContactsTable(contacts []graph.Contact, username string) error {
	rows := make([][]string, 0, len(contacts))
	for _, contact := range contacts {
		email := ""
		if len(contact.EmailAddresses) > 0 {
			email = contact.EmailAddresses[0].Address
		}
		phone := contact.MobilePhone
		if phone == "" && len(contact.BusinessPhones) > 0 {
			phone = contact.BusinessPhones[0]
		}
		rows = append(rows, []string{contact.DisplayName, email, contact.CompanyName, phone})
	}
	fmt.Fprintf(os.Stdout, "账号: %s\n", username)
	return cliui.PrintTable(os.Stdout, []string{"姓名", "邮箱", "公司", "电话"}, rows)
}

func printJSONValue(value any) error {
	return printJSON(value)
}

func printOutputValue(value any, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return printJSON(value)
	case "yaml":
		data, err := yaml.Marshal(value)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stdout, string(data))
		return nil
	case "jsonl":
		rv := reflect.ValueOf(value)
		if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
			for i := 0; i < rv.Len(); i++ {
				data, err := json.Marshal(rv.Index(i).Interface())
				if err != nil {
					return err
				}
				fmt.Fprintln(os.Stdout, string(data))
			}
			return nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	default:
		return fmt.Errorf("不支持的输出格式: %s", format)
	}
}
