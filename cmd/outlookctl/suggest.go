package main

import "strings"

var commandNames = []string{"auth", "account", "audit", "mailbox", "doctor", "update", "config", "completion", "ui", "sync", "search", "mail", "calendar", "contacts", "version", "help"}

func suggestCommand(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	best := ""
	bestDistance := 4
	for _, command := range commandNames {
		distance := levenshtein(input, command)
		if distance < bestDistance {
			bestDistance = distance
			best = command
		}
	}
	return best
}

func levenshtein(left, right string) int {
	a := []rune(left)
	b := []rune(right)
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = minInt(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

func minInt(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
