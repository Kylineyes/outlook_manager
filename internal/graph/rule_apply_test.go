package graph

import (
	"reflect"
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
	if !matchesMessageRule(conditions, message) {
		t.Fatal("expected message to match")
	}
	message.Importance = "normal"
	if matchesMessageRule(conditions, message) {
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
