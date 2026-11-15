package semantic

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	if got := cosineSimilarity([]float64{1, 0}, []float64{1, 0}); math.Abs(got-1) > 1e-9 {
		t.Fatalf("identical vectors = %v", got)
	}
	if got := cosineSimilarity([]float64{1, 0}, []float64{0, 1}); math.Abs(got) > 1e-9 {
		t.Fatalf("orthogonal vectors = %v", got)
	}
	if got := cosineSimilarity([]float64{1}, []float64{1, 2}); got != 0 {
		t.Fatalf("mismatched vectors = %v", got)
	}
	if got := cosineSimilarity([]float64{0, 0}, []float64{1, 1}); got != 0 {
		t.Fatalf("zero vector = %v", got)
	}
}
