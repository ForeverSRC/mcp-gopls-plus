package memory

import "math"

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

func bm25Score(tf, docFreq, docCount, docLen int, avgDocLen float64) float64 {
	if tf <= 0 || docFreq <= 0 || docCount <= 0 || avgDocLen <= 0 {
		return 0
	}

	idf := math.Log(1 + (float64(docCount-docFreq)+0.5)/(float64(docFreq)+0.5))
	numerator := float64(tf) * (bm25K1 + 1)
	denominator := float64(tf) + bm25K1*(1-bm25B+bm25B*(float64(docLen)/avgDocLen))
	if denominator == 0 {
		return 0
	}
	return idf * (numerator / denominator)
}
