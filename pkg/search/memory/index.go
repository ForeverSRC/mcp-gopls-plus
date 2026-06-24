package memory

import (
	"math"

	"github.com/ForeverSRC/mcp-gopls-plus/pkg/search"
)

type posting struct {
	entryID int
	tf      int
}

type indexedEntry struct {
	chunk     search.Chunk
	tokenFreq map[string]int
	docLen    int
}

type index struct {
	entries   []indexedEntry
	postings  map[string][]posting
	docFreq   map[string]int
	avgDocLen float64
}

func buildIndex(chunks []search.Chunk) *index {
	idx := &index{
		entries:  make([]indexedEntry, 0, len(chunks)),
		postings: make(map[string][]posting),
		docFreq:  make(map[string]int),
	}

	totalDocLen := 0
	for _, chunk := range chunks {
		freq := make(map[string]int)
		for _, token := range chunk.Tokens {
			freq[token]++
		}
		if len(freq) == 0 {
			continue
		}

		entry := indexedEntry{
			chunk:     chunk,
			tokenFreq: freq,
			docLen:    len(chunk.Tokens),
		}
		entryID := len(idx.entries)
		idx.entries = append(idx.entries, entry)
		totalDocLen += entry.docLen

		for token, tf := range freq {
			idx.postings[token] = append(idx.postings[token], posting{
				entryID: entryID,
				tf:      tf,
			})
			idx.docFreq[token]++
		}
	}

	if len(idx.entries) > 0 {
		idx.avgDocLen = float64(totalDocLen) / float64(len(idx.entries))
	}
	return idx
}

type scoredEntry struct {
	entry indexedEntry
	score float64
}

func (idx *index) score(queryTokens []string) []scoredEntry {
	if idx == nil || len(idx.entries) == 0 || len(queryTokens) == 0 {
		return nil
	}

	acc := make(map[int]float64)
	seen := dedupeStrings(queryTokens)
	for _, token := range seen {
		for _, post := range idx.postings[token] {
			acc[post.entryID] += bm25Score(post.tf, idx.docFreq[token], len(idx.entries), idx.entries[post.entryID].docLen, idx.avgDocLen)
		}
	}

	results := make([]scoredEntry, 0, len(acc))
	for entryID, score := range acc {
		if score <= 0 || math.IsNaN(score) || math.IsInf(score, 0) {
			continue
		}
		results = append(results, scoredEntry{
			entry: idx.entries[entryID],
			score: score,
		})
	}
	return results
}
