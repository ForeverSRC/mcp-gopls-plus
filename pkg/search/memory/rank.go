package memory

import (
	"math"
	"strings"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/search"
)

func rerankResults(results []scoredEntry, queryTokens []string, opts search.Options) {
	if len(results) == 0 {
		return
	}

	fileMatches := make(map[string]int)
	for _, item := range results {
		fileMatches[item.entry.chunk.File]++
	}

	for i := range results {
		chunk := results[i].entry.chunk
		if chunk.IsTest && !opts.IncludeTests {
			results[i].score = -1
			continue
		}

		boost := 1.0
		boost += stemOverlapBoost(queryTokens, chunk.SymbolTokens)
		boost += fileCoherenceBoost(fileMatches[chunk.File])

		if chunk.Kind != "file" {
			boost += 0.12
		}
		if chunk.IsNoise {
			boost -= 0.18
		}
		if chunk.IsTest {
			boost -= 0.08
		}
		if strings.HasSuffix(strings.ToLower(chunk.File), ".pb.go") {
			boost -= 0.12
		}

		if boost < 0.05 {
			boost = 0.05
		}
		results[i].score *= boost
	}
}

func stemOverlapBoost(queryTokens, symbolTokens []string) float64 {
	if len(queryTokens) == 0 || len(symbolTokens) == 0 {
		return 0
	}

	symbolSet := make(map[string]struct{}, len(symbolTokens))
	for _, token := range symbolTokens {
		symbolSet[token] = struct{}{}
	}

	matches := 0
	for _, token := range dedupeStrings(queryTokens) {
		if _, ok := symbolSet[token]; ok {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}

	return math.Min(0.25, float64(matches)/float64(len(symbolSet))*0.2)
}

func fileCoherenceBoost(matchCount int) float64 {
	if matchCount <= 1 {
		return 0
	}
	return math.Min(0.12, float64(matchCount-1)*0.03)
}

func normalizeScores(results []scoredEntry) {
	maxScore := 0.0
	for _, item := range results {
		if item.score > maxScore {
			maxScore = item.score
		}
	}
	if maxScore <= 0 {
		for i := range results {
			results[i].score = 0
		}
		return
	}

	for i := range results {
		score := results[i].score
		if score < 0 {
			score = 0
		}
		results[i].score = math.Round((score/maxScore)*1000) / 1000
	}
}
