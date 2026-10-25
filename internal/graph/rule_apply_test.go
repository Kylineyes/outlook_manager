package graph

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildMessageRulePayloadIncludesAssignCategories(t *testing.T) {
	payload, err := buildMessageRulePayload(MessageRuleInput{
		DisplayName:      "分类规则",
		SubjectContains:  []string{"invoice"},
		AssignCategories: []string{"项目A", "重要"},
		IsEnabled:        true,
	})
	if err != nil {
		t.Fatalf("buildMessageRulePayload returned error: %v", err)
	}
	actions, ok := payload["actions"].(map[string]any)
	if !ok {
		t.Fatalf("actions has unexpected type: %T", payload["actions"])
	}
	got, ok := actions["assignCategories"].([]string)
	if !ok {
		t.Fatalf("assignCategories has unexpected type: %T", actions["assignCategories"])
	}
	want := []string{"项目A", "重要"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assignCategories = %#v, want %#v", got, want)
	}
}

func TestMatchesMessageRule(t *testing.T) {
	message := Message{
		Subject:        "Your invoice is ready",
		BodyPreview:    "Please find the invoice attached.",
		HasAttachments: true,
		Importance:     "high",
		From:           MessageFrom{EmailAddress: EmailAddress{Address: "billing@example.com"}},
	}
	conditions := &messageRuleConditions{
		FromAddresses:   []string{"BILLING@example.com"},
		SubjectContains: []string{"invoice"},
		HasAttachments:  true,
		Importance:      "high",
	}
	if !matchesMessageRule(conditions, message, message.BodyPreview) {
		t.Fatal("expected message to match")
	}
	message.Importance = "normal"
	if matchesMessageRule(conditions, message, message.BodyPreview) {
		t.Fatal("expected message not to match after importance changed")
	}
}

func TestParseMessageRuleActionsSupportsCategories(t *testing.T) {
	actions, err := parseMessageRuleActions(map[string]any{
		"assignCategories": []any{"项目A", "重要"},
		"markAsRead":       true,
	})
	if err != nil {
		t.Fatalf("parseMessageRuleActions returned error: %v", err)
	}
	if !actions.MarkAsRead {
		t.Fatal("expected MarkAsRead true")
	}
	if !reflect.DeepEqual(actions.AssignCategories, []string{"项目A", "重要"}) {
		t.Fatalf("AssignCategories = %#v", actions.AssignCategories)
	}
}

func TestParseMessageRuleActionsRejectsUnsupportedAction(t *testing.T) {
	_, err := parseMessageRuleActions(map[string]any{"forwardTo": []any{"someone@example.com"}})
	if err == nil {
		t.Fatal("expected error for unsupported action")
	}
}

func TestMatchesMessageRuleUsesFullBody(t *testing.T) {
	message := Message{Subject: "hello", BodyPreview: "preview only"}
	conditions := &messageRuleConditions{BodyContains: []string{"完整正文关键字"}}
	if matchesMessageRule(conditions, message, "这是一段完整正文关键字内容") {
		return
	}
	t.Fatal("expected full body to match")
}

func TestParseMessageRuleActionsSupportsCopyAndForward(t *testing.T) {
	actions, err := parseMessageRuleActions(map[string]any{
		"copyToFolder": "folder-copy",
		"forwardTo": []any{
			map[string]any{"emailAddress": map[string]any{"address": "a@example.com"}},
		},
		"redirectTo": []any{
			map[string]any{"emailAddress": map[string]any{"address": "b@example.com"}},
		},
	})
	if err != nil {
		t.Fatalf("parseMessageRuleActions returned error: %v", err)
	}
	if actions.CopyToFolder != "folder-copy" {
		t.Fatalf("CopyToFolder = %q", actions.CopyToFolder)
	}
	if !reflect.DeepEqual(actions.ForwardTo, []string{"a@example.com"}) {
		t.Fatalf("ForwardTo = %#v", actions.ForwardTo)
	}
	if !reflect.DeepEqual(actions.RedirectTo, []string{"b@example.com"}) {
		t.Fatalf("RedirectTo = %#v", actions.RedirectTo)
	}
}

func TestHTMLToText(t *testing.T) {
	got := htmlToText("<p>Hello &amp; world</p><br><b>test</b>")
	if !strings.Contains(got, "Hello & world") || !strings.Contains(got, "test") {
		t.Fatalf("htmlToText = %q", got)
	}
}
