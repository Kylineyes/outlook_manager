package main

import (
	"os"
	"regexp"
	"testing"
)

func TestToolRegistrationContract(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`addTool\(server, newTool\("([^"]+)"`)
	names := map[string]bool{}
	count := 0
	for _, file := range files {
		if file.IsDir() || file.Name() == "contract_test.go" || !regexp.MustCompile(`\.go$`).MatchString(file.Name()) {
			continue
		}
		data, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			name := match[1]
			if name == "" {
				t.Fatal("empty tool name")
			}
			if names[name] {
				t.Fatalf("duplicate tool name: %s", name)
			}
			names[name] = true
			count++
		}
	}
	if count != 118 {
		t.Fatalf("tool count = %d, want 118", count)
	}
}
