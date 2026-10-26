package graph

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMockGraphMailAndCalendarFlow(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	var sentBody string
	var scheduleBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.0/me/sendMail":
			data, _ := io.ReadAll(r.Body)
			sentBody = string(data)
			w.WriteHeader(http.StatusAccepted)
		case r.URL.Path == "/v1.0/me/messages":
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`{"value":[{"id":"m2","subject":"second"}]}`))
				return
			}
			next := "http://" + r.Host + "/v1.0/me/messages?page=2"
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []Message{{ID: "m1", Subject: "first"}}, "@odata.nextLink": next})
		case r.URL.Path == "/v1.0/me/calendar/getSchedule":
			data, _ := io.ReadAll(r.Body)
			scheduleBody = string(data)
			_, _ = w.Write([]byte(`{"value":[{"scheduleId":"a@example.com","availabilityView":"00","scheduleItems":[]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oldBase, oldClient := graphBaseURL, httpClient
	graphBaseURL = server.URL + "/v1.0"
	httpClient = server.Client()
	defer func() { graphBaseURL, httpClient = oldBase, oldClient }()

	err := SendMail(context.Background(), "token", SendMailInput{
		To: []string{"to@example.com"}, Subject: "integration", Body: "body",
		Attachments: []MailAttachmentInput{{Name: "a.txt", ContentType: "text/plain", ContentBytes: []byte("hello"), Size: 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sentBody, `"subject":"integration"`) || !strings.Contains(sentBody, `"name":"a.txt"`) {
		t.Fatalf("unexpected send body: %s", sentBody)
	}

	messages, err := ListAllMessagesAll(context.Background(), "token", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != "m1" || messages[1].ID != "m2" {
		t.Fatalf("unexpected messages: %#v", messages)
	}

	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	schedules, err := GetSchedule(context.Background(), "token", []string{"a@example.com"}, start, start.Add(time.Hour), 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 1 || !strings.Contains(scheduleBody, `"availabilityViewInterval":30`) {
		t.Fatalf("unexpected schedule: %#v body=%s", schedules, scheduleBody)
	}
}
