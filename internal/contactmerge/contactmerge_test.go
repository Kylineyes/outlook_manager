package contactmerge

import (
	"testing"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func TestFindAndMergeByEmail(t *testing.T) {
	contacts := []graph.Contact{
		{ID: "1", DisplayName: "张三", EmailAddresses: []graph.ContactEmailAddress{{Address: "A@example.com"}}},
		{ID: "2", CompanyName: "示例公司", EmailAddresses: []graph.ContactEmailAddress{{Address: "a@example.com"}}, BusinessPhones: []string{"010-12345678"}},
		{ID: "3", DisplayName: "李四", EmailAddresses: []graph.ContactEmailAddress{{Address: "b@example.com"}}},
	}
	groups := FindGroups(contacts, "email")
	if len(groups) != 1 || len(groups[0].Contacts) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
	master, duplicateIDs := MergeGroup(groups[0])
	if master.DisplayName != "张三" || master.CompanyName != "示例公司" {
		t.Fatalf("unexpected master: %#v", master)
	}
	if len(duplicateIDs) != 1 || duplicateIDs[0] != "2" {
		t.Fatalf("duplicate ids = %#v", duplicateIDs)
	}
}

func TestFindByPhoneAndName(t *testing.T) {
	contacts := []graph.Contact{
		{ID: "1", DisplayName: "张三", MobilePhone: "138-0000-0000"},
		{ID: "2", DisplayName: "张三", BusinessPhones: []string{"13800000000"}},
	}
	if groups := FindGroups(contacts, "phone"); len(groups) != 1 {
		t.Fatalf("phone groups = %#v", groups)
	}
	if groups := FindGroups(contacts, "name"); len(groups) != 1 {
		t.Fatalf("name groups = %#v", groups)
	}
}
