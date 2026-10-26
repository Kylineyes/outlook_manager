// Package contactmerge 负责联系人重复检测和合并。
package contactmerge

import (
	"sort"
	"strings"
	"unicode"

	"github.com/Kylineyes/outlook-manager/internal/graph"
)

// Group 表示一组重复联系人。
type Group struct {
	Key      string
	Strategy string
	Contacts []graph.Contact
}

// FindGroups 按指定策略查找重复联系人。
func FindGroups(contacts []graph.Contact, strategy string) []Group {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy == "" {
		strategy = "email"
	}
	parent := make([]int, len(contacts))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	keyed := map[string][]int{}
	for index, contact := range contacts {
		for _, key := range contactKeys(contact, strategy) {
			keyed[key] = append(keyed[key], index)
		}
	}
	for _, indexes := range keyed {
		for i := 1; i < len(indexes); i++ {
			union(indexes[0], indexes[i])
		}
	}
	grouped := map[int][]int{}
	for index := range contacts {
		root := find(index)
		grouped[root] = append(grouped[root], index)
	}
	groups := make([]Group, 0)
	for _, indexes := range grouped {
		if len(indexes) < 2 {
			continue
		}
		key := ""
		for _, candidate := range contactKeys(contacts[indexes[0]], strategy) {
			count := 0
			for _, index := range indexes {
				for _, other := range contactKeys(contacts[index], strategy) {
					if other == candidate {
						count++
						break
					}
				}
			}
			if count >= 2 {
				key = candidate
				break
			}
		}
		members := make([]graph.Contact, 0, len(indexes))
		for _, index := range indexes {
			members = append(members, contacts[index])
		}
		sort.SliceStable(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		groups = append(groups, Group{Key: key, Strategy: strategy, Contacts: members})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Key != groups[j].Key {
			return groups[i].Key < groups[j].Key
		}
		return groups[i].Contacts[0].ID < groups[j].Contacts[0].ID
	})
	return groups
}

func contactKeys(contact graph.Contact, strategy string) []string {
	keys := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			keys = append(keys, value)
		}
	}
	switch strategy {
	case "email":
		for _, email := range contact.EmailAddresses {
			add(normalizeEmail(email.Address))
		}
	case "phone":
		add(normalizePhone(contact.MobilePhone))
		for _, phone := range contact.BusinessPhones {
			add(normalizePhone(phone))
		}
	case "name":
		add(normalizeName(contact.DisplayName))
		if strings.TrimSpace(contact.DisplayName) == "" {
			add(normalizeName(contact.GivenName + contact.Surname))
		}
	}
	return keys
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizePhone(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	result := builder.String()
	if len(result) < 7 {
		return ""
	}
	return result
}

func normalizeName(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// MergeGroup 合并一组重复联系人，返回主联系人和待删除的重复联系人 ID。
func MergeGroup(group Group) (graph.Contact, []string) {
	if len(group.Contacts) == 0 {
		return graph.Contact{}, nil
	}
	masterIndex := 0
	bestScore := -1
	for index, contact := range group.Contacts {
		score := contactScore(contact)
		if score > bestScore {
			bestScore = score
			masterIndex = index
		}
	}
	master := group.Contacts[masterIndex]
	duplicateIDs := make([]string, 0, len(group.Contacts)-1)
	for index, contact := range group.Contacts {
		if index == masterIndex {
			continue
		}
		mergeInto(&master, contact)
		if strings.TrimSpace(contact.ID) != "" {
			duplicateIDs = append(duplicateIDs, contact.ID)
		}
	}
	for i := range master.EmailAddresses {
		master.EmailAddresses[i].Address = strings.TrimSpace(master.EmailAddresses[i].Address)
	}
	return master, duplicateIDs
}

func contactScore(contact graph.Contact) int {
	score := 0
	for _, value := range []string{
		contact.DisplayName, contact.GivenName, contact.Surname, contact.CompanyName,
		contact.JobTitle, contact.MobilePhone, contact.NickName, contact.Birthday,
		contact.PersonalNotes, contact.Department, contact.OfficeLocation,
		contact.Profession, contact.BusinessHomePage,
	} {
		if strings.TrimSpace(value) != "" {
			score += 2
		}
	}
	if strings.TrimSpace(contact.DisplayName) != "" {
		score += 3
	}
	score += len(contact.EmailAddresses) * 3
	score += len(contact.BusinessPhones) * 2
	if contact.HomeAddress != nil {
		score++
	}
	if contact.BusinessAddress != nil {
		score++
	}
	if contact.OtherAddress != nil {
		score++
	}
	return score
}

func mergeInto(master *graph.Contact, other graph.Contact) {
	fill := func(target *string, value string) {
		if strings.TrimSpace(*target) == "" {
			*target = value
		}
	}
	fill(&master.DisplayName, other.DisplayName)
	fill(&master.GivenName, other.GivenName)
	fill(&master.Surname, other.Surname)
	fill(&master.CompanyName, other.CompanyName)
	fill(&master.JobTitle, other.JobTitle)
	fill(&master.MobilePhone, other.MobilePhone)
	fill(&master.NickName, other.NickName)
	fill(&master.Birthday, other.Birthday)
	fill(&master.PersonalNotes, other.PersonalNotes)
	fill(&master.Department, other.Department)
	fill(&master.OfficeLocation, other.OfficeLocation)
	fill(&master.Profession, other.Profession)
	fill(&master.BusinessHomePage, other.BusinessHomePage)
	if master.HomeAddress == nil {
		master.HomeAddress = other.HomeAddress
	}
	if master.BusinessAddress == nil {
		master.BusinessAddress = other.BusinessAddress
	}
	if master.OtherAddress == nil {
		master.OtherAddress = other.OtherAddress
	}
	seenEmails := map[string]bool{}
	for _, email := range master.EmailAddresses {
		seenEmails[normalizeEmail(email.Address)] = true
	}
	for _, email := range other.EmailAddresses {
		key := normalizeEmail(email.Address)
		if key != "" && !seenEmails[key] {
			master.EmailAddresses = append(master.EmailAddresses, email)
			seenEmails[key] = true
		}
	}
	seenPhones := map[string]bool{}
	for _, phone := range master.BusinessPhones {
		seenPhones[normalizePhone(phone)] = true
	}
	for _, phone := range other.BusinessPhones {
		key := normalizePhone(phone)
		if key != "" && !seenPhones[key] {
			master.BusinessPhones = append(master.BusinessPhones, phone)
			seenPhones[key] = true
		}
	}
}
