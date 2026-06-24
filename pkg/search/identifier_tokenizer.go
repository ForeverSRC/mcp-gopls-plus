package search

import (
	"strings"
	"unicode"
)

func tokenizeText(text string) []string {
	text = strings.ReplaceAll(text, "/", " ")
	text = strings.ReplaceAll(text, ".", " ")
	text = strings.ReplaceAll(text, "-", " ")

	rawParts := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})

	var tokens []string
	for _, part := range rawParts {
		tokens = append(tokens, tokenizeIdentifier(part)...)
	}
	return tokens
}

func tokenizeIdentifier(input string) []string {
	if input == "" {
		return nil
	}

	var parts []string
	var current []rune
	runes := []rune(input)

	flush := func() {
		if len(current) == 0 {
			return
		}
		token := strings.ToLower(strings.TrimSpace(string(current)))
		if token != "" {
			parts = append(parts, token)
		}
		current = current[:0]
	}

	for i, r := range runes {
		if r == '_' || r == '-' || r == '.' || unicode.IsSpace(r) {
			flush()
			continue
		}

		if i > 0 {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsUpper(r) && (unicode.IsLower(prev) || nextLower) {
				flush()
			}
		}
		current = append(current, r)
	}
	flush()

	return dedupeStrings(parts)
}

func dedupeStrings(items []string) []string {
	if len(items) <= 1 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
