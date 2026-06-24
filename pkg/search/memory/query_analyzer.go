package memory

import (
	"strings"
	"unicode"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/search"
)

var stopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {},
	"for": {}, "from": {}, "go": {}, "in": {}, "into": {}, "is": {}, "of": {},
	"on": {}, "or": {}, "returns": {}, "the": {}, "to": {}, "with": {},
}

type queryAnalyzer struct{}

var _ search.QueryAnalyzer = queryAnalyzer{}

func newQueryAnalyzer() search.QueryAnalyzer {
	return queryAnalyzer{}
}

func (queryAnalyzer) Analyze(query string) []string {
	return tokenizeQuery(query)
}

func tokenizeQuery(query string) []string {
	tokens := tokenizeText(query)
	for i, token := range tokens {
		tokens[i] = normalizeToken(token)
	}

	filtered := tokens[:0]
	for _, token := range tokens {
		if token == "" {
			continue
		}
		filtered = append(filtered, token)
	}
	return dedupeStrings(filtered)
}

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

func normalizeToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if _, ok := stopWords[token]; ok {
		return ""
	}
	token = trimSuffix(token)
	token = normalizeAlias(token)
	if len(token) <= 1 {
		return ""
	}
	if _, ok := stopWords[token]; ok {
		return ""
	}
	return token
}

func trimSuffix(token string) string {
	switch {
	case len(token) > 5 && strings.HasSuffix(token, "ing"):
		root := strings.TrimSuffix(token, "ing")
		if strings.HasSuffix(root, "l") {
			return root + "e"
		}
		return root
	case len(token) > 4 && strings.HasSuffix(token, "ed"):
		return strings.TrimSuffix(token, "ed")
	case len(token) > 4 && strings.HasSuffix(token, "es"):
		return strings.TrimSuffix(token, "es")
	case len(token) > 3 && strings.HasSuffix(token, "s"):
		return strings.TrimSuffix(token, "s")
	default:
		return token
	}
}

func normalizeAlias(token string) string {
	switch token {
	case "authentication", "authenticate", "authenticat", "authorization", "authoriz", "auth":
		return "auth"
	case "handling", "handler", "handles", "handled":
		return "handle"
	case "configuration":
		return "config"
	default:
		return token
	}
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
