package vcard

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

func TestVCardRoundTrip(t *testing.T) {
	contacts := []graph.Contact{{
		DisplayName:      "张三",
		GivenName:        "三",
		Surname:          "张",
		EmailAddresses:   []graph.ContactEmailAddress{{Address: "a@example.com"}},
		CompanyName:      "示例公司",
		JobTitle:         "工程师",
		MobilePhone:      "13800000000",
		BusinessPhones:   []string{"010-12345678"},
		NickName:         "老张",
		Birthday:         "1990-01-02T00:00:00Z",
		PersonalNotes:    "备注",
		BusinessHomePage: "https://example.com",
		HomeAddress:      &graph.PhysicalAddress{Street: "家庭路1号", City: "上海", State: "上海", PostalCode: "200000", CountryOrRegion: "中国"},
	}}
	var buffer bytes.Buffer
	if err := WriteContacts(&buffer, contacts); err != nil {
		t.Fatal(err)
	}
	inputs, err := ReadContacts(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 {
		t.Fatalf("contacts = %d, want 1", len(inputs))
	}
	input := inputs[0]
	if input.DisplayName != "张三" || input.GivenName != "三" || input.Surname != "张" {
		t.Fatalf("unexpected name: %#v", input)
	}
	if !reflect.DeepEqual(input.BusinessPhones, []string{"010-12345678"}) {
		t.Fatalf("unexpected phones: %#v", input.BusinessPhones)
	}
	if input.HomeAddress == nil || input.HomeAddress.City != "上海" {
		t.Fatalf("unexpected address: %#v", input.HomeAddress)
	}
}
