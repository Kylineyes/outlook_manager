// Package ics 负责 iCalendar (.ics) 的导入和导出。
package ics

import (
	"bufio"
	"fmt"
	"io"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

// WriteEvents 把日历事件写成 iCalendar 3.0 文本。
func WriteEvents(w io.Writer, events []graph.CalendarEvent) error {
	writer := bufio.NewWriter(w)
	fmt.Fprintln(writer, "BEGIN:VCALENDAR")
	fmt.Fprintln(writer, "VERSION:2.0")
	fmt.Fprintln(writer, "PRODID:-//Outlook Manager//CN")
	fmt.Fprintln(writer, "CALSCALE:GREGORIAN")
	for _, event := range events {
		fmt.Fprintln(writer, "BEGIN:VEVENT")
		uid := strings.TrimSpace(event.ICalUId)
		if uid == "" {
			uid = event.ID
		}
		fmt.Fprintf(writer, "UID:%s\n", escapeText(uid))
		fmt.Fprintf(writer, "DTSTAMP:%s\n", time.Now().UTC().Format("20060102T150405Z"))
		writeDateProperty(writer, "DTSTART", event.Start, event.IsAllDay)
		writeDateProperty(writer, "DTEND", event.End, event.IsAllDay)
		writeTextProperty(writer, "SUMMARY", event.Subject)
		if event.Body.Content != "" {
			writeTextProperty(writer, "DESCRIPTION", event.Body.Content)
		}
		if event.Location.DisplayName != "" {
			writeTextProperty(writer, "LOCATION", event.Location.DisplayName)
		}
		if len(event.Categories) > 0 {
			fmt.Fprintf(writer, "CATEGORIES:%s\n", escapeText(strings.Join(event.Categories, ",")))
		}
		if event.Recurrence != nil {
			fmt.Fprintf(writer, "RRULE:%s\n", formatRRule(*event.Recurrence))
		}
		for _, attendee := range event.Attendees {
			if attendee.EmailAddress.Address == "" {
				continue
			}
			fmt.Fprintf(writer, "ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION:mailto:%s\n", attendee.EmailAddress.Address)
		}
		if event.ShowAs != "" {
			fmt.Fprintf(writer, "X-MICROSOFT-CDO-BUSYSTATUS:%s\n", strings.ToUpper(event.ShowAs))
		}
		if event.Importance != "" {
			priority := "5"
			switch strings.ToLower(event.Importance) {
			case "high":
				priority = "1"
			case "low":
				priority = "9"
			}
			fmt.Fprintf(writer, "PRIORITY:%s\n", priority)
		}
		fmt.Fprintln(writer, "END:VEVENT")
	}
	fmt.Fprintln(writer, "END:VCALENDAR")
	return writer.Flush()
}

func writeDateProperty(writer io.Writer, name string, value graph.DateTimeTimeZone, allDay bool) {
	if allDay {
		if parsed, err := parseGraphTime(value); err == nil {
			fmt.Fprintf(writer, "%s;VALUE=DATE:%s\n", name, parsed.Format("20060102"))
			return
		}
	}
	if parsed, err := parseGraphTime(value); err == nil {
		fmt.Fprintf(writer, "%s:%s\n", name, parsed.UTC().Format("20060102T150405Z"))
		return
	}
	fmt.Fprintf(writer, "%s:%s\n", name, strings.ReplaceAll(value.DateTime, "-", ""))
}

func writeTextProperty(writer io.Writer, name, value string) {
	if value == "" {
		return
	}
	folded := escapeText(value)
	fmt.Fprintf(writer, "%s:%s\n", name, folded)
}

func parseGraphTime(value graph.DateTimeTimeZone) (time.Time, error) {
	raw := strings.TrimSpace(value.DateTime)
	layouts := []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, raw, time.UTC)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", value.DateTime)
}

func formatRRule(rule graph.Recurrence) string {
	parts := []string{"FREQ=" + strings.ToUpper(rule.Pattern.Type)}
	if rule.Pattern.Interval > 0 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(rule.Pattern.Interval))
	}
	if len(rule.Pattern.DaysOfWeek) > 0 {
		days := make([]string, 0, len(rule.Pattern.DaysOfWeek))
		for _, day := range rule.Pattern.DaysOfWeek {
			days = append(days, strings.ToUpper(day[:2]))
		}
		parts = append(parts, "BYDAY="+strings.Join(days, ","))
	}
	if rule.Pattern.DayOfMonth > 0 {
		parts = append(parts, "BYMONTHDAY="+strconv.Itoa(rule.Pattern.DayOfMonth))
	}
	if rule.Pattern.Month > 0 {
		parts = append(parts, "BYMONTH="+strconv.Itoa(rule.Pattern.Month))
	}
	if rule.Range.NumberOfOccurrences > 0 {
		parts = append(parts, "COUNT="+strconv.Itoa(rule.Range.NumberOfOccurrences))
	} else if rule.Range.EndDate != "" {
		parts = append(parts, "UNTIL="+strings.ReplaceAll(rule.Range.EndDate, "-", "")+"T235959Z")
	}
	return strings.Join(parts, ";")
}

// Parse 从 iCalendar 文本解析事件。
func Parse(r io.Reader) ([]graph.CalendarEventInput, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), 4*1024*1024)
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
		return nil, fmt.Errorf("读取 ICS 失败: %w", err)
	}
	events := make([]graph.CalendarEventInput, 0)
	var current *graph.CalendarEventInput
	var startValue, endValue, durationValue string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "BEGIN:VEVENT") {
			current = &graph.CalendarEventInput{}
			startValue, endValue, durationValue = "", "", ""
			continue
		}
		if strings.EqualFold(line, "END:VEVENT") {
			if current != nil {
				if err := finalizeEvent(current, startValue, endValue, durationValue); err != nil {
					return nil, err
				}
				if strings.TrimSpace(current.Subject) != "" || !current.Start.IsZero() {
					events = append(events, *current)
				}
			}
			current = nil
			continue
		}
		if current == nil {
			continue
		}
		name, params, value, ok := splitProperty(line)
		if !ok {
			continue
		}
		value = unescapeText(value)
		switch strings.ToUpper(name) {
		case "SUMMARY":
			current.Subject = value
		case "DESCRIPTION":
			current.Body = value
		case "LOCATION":
			current.Location = value
		case "DTSTART":
			startValue = value
			current.TimeZone = params["TZID"]
			current.IsAllDay = strings.EqualFold(params["VALUE"], "DATE")
		case "DTEND":
			endValue = value
		case "DURATION":
			durationValue = value
		case "RRULE":
			applyRRule(current, value)
		case "CATEGORIES":
			for _, category := range strings.Split(value, ",") {
				if strings.TrimSpace(category) != "" {
					current.Categories = append(current.Categories, strings.TrimSpace(category))
				}
			}
		case "ATTENDEE":
			address := strings.TrimPrefix(value, "mailto:")
			if address != "" {
				current.Attendees = append(current.Attendees, address)
			}
		case "X-MICROSOFT-CDO-BUSYSTATUS":
			current.ShowAs = strings.ToLower(value)
		case "PRIORITY":
			if number, err := strconv.Atoi(value); err == nil {
				if number <= 4 {
					current.Importance = "high"
				} else if number >= 6 {
					current.Importance = "low"
				} else {
					current.Importance = "normal"
				}
			}
		case "CLASS":
			current.Sensitivity = strings.ToLower(value)
		}
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("ICS 文件中没有有效事件")
	}
	return events, nil
}

func finalizeEvent(event *graph.CalendarEventInput, startValue, endValue, durationValue string) error {
	if strings.TrimSpace(startValue) == "" {
		return fmt.Errorf("VEVENT 缺少 DTSTART")
	}
	timeZone := event.TimeZone
	if strings.TrimSpace(timeZone) == "" {
		timeZone = "UTC"
	}
	start, allDay, err := parseICSDate(startValue, timeZone, event.IsAllDay)
	if err != nil {
		return err
	}
	event.Start = start
	event.IsAllDay = allDay
	if strings.TrimSpace(endValue) != "" {
		end, _, err := parseICSDate(endValue, timeZone, allDay)
		if err != nil {
			return err
		}
		event.End = end
	} else if durationValue != "" {
		duration, err := parseDuration(durationValue)
		if err != nil {
			return err
		}
		event.End = start.Add(duration)
	} else if allDay {
		event.End = start.AddDate(0, 0, 1)
	} else {
		event.End = start.Add(time.Hour)
	}
	return nil
}

func parseICSDate(value, timeZone string, allDay bool) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	if allDay || len(value) == 8 {
		parsed, err := time.ParseInLocation("20060102", value, loadLocation(timeZone))
		return parsed, true, err
	}
	if strings.HasSuffix(value, "Z") {
		parsed, err := time.ParseInLocation("20060102T150405Z", value, time.UTC)
		return parsed, false, err
	}
	location := loadLocation(timeZone)
	for _, layout := range []string{"20060102T150405", "20060102T1504"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("无法解析 ICS 时间: %s", value)
}

func loadLocation(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return location
}

func parseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "PT") && strings.HasSuffix(value, "M") {
		minutes, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(value, "PT"), "M"))
		if err != nil {
			return 0, err
		}
		return time.Duration(minutes) * time.Minute, nil
	}
	if strings.HasPrefix(value, "P") && strings.HasSuffix(value, "D") {
		days, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(value, "P"), "D"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("不支持的 DURATION: %s", value)
}

func applyRRule(event *graph.CalendarEventInput, value string) {
	for _, part := range strings.Split(value, ";") {
		pairs := strings.SplitN(part, "=", 2)
		if len(pairs) != 2 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(pairs[0]))
		value := strings.TrimSpace(pairs[1])
		switch key {
		case "FREQ":
			event.RepeatType = strings.ToLower(value)
		case "INTERVAL":
			if number, err := strconv.Atoi(value); err == nil {
				event.RepeatInterval = number
			}
		case "COUNT":
			if number, err := strconv.Atoi(value); err == nil {
				event.RepeatCount = number
			}
		case "UNTIL":
			event.RepeatUntil = value
		}
	}
}

func splitProperty(line string) (string, map[string]string, string, bool) {
	index := strings.Index(line, ":")
	if index < 0 {
		return "", nil, "", false
	}
	left := line[:index]
	value := line[index+1:]
	parts := strings.Split(left, ";")
	params := map[string]string{}
	for _, part := range parts[1:] {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) == 2 {
			params[strings.ToUpper(pair[0])] = pair[1]
		}
	}
	return parts[0], params, value, true
}

func escapeText(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\r\n", "\\n", "\n", "\\n")
	return replacer.Replace(value)
}

func unescapeText(value string) string {
	replacer := strings.NewReplacer("\\n", "\n", "\\N", "\n", "\\,", ",", "\\;", ";", "\\\\", "\\")
	return replacer.Replace(value)
}

// ParseAddress 保留给调用方做地址友好显示。
func ParseAddress(value string) string {
	if address, err := mail.ParseAddress(value); err == nil {
		return address.Address
	}
	return value
}
