package localmodels

import (
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"math"
	"strings"
)

func embeddingText(t *pb.TextInput, task pb.EmbeddingTask) string {
	switch task {
	case pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT:
		title := t.Title
		if strings.TrimSpace(title) == "" {
			title = "none"
		}
		return "title: " + title + " | text: " + t.Text
	case pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY:
		return "task: code retrieval | query: " + t.Text
	default:
		return "task: search result | query: " + t.Text
	}
}
func normalizeVector(raw []float64, native, dimensions int) ([]float32, error) {
	if len(raw) != native || dimensions > native || dimensions == 0 {
		return nil, fault(Protocol)
	}
	// Validate every native element before truncation, including the discarded tail.
	for _, v := range raw {
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, fault(Protocol)
		}
	}
	norm := float64(0)
	for _, v := range raw[:dimensions] {
		norm += v * v
	}
	if norm <= 0 || math.IsInf(norm, 0) || math.IsNaN(norm) {
		return nil, fault(Protocol)
	}
	norm = math.Sqrt(norm)
	vector := make([]float32, dimensions)
	for i := range vector {
		vector[i] = float32(raw[i] / norm)
		if math.IsInf(float64(vector[i]), 0) || math.IsNaN(float64(vector[i])) {
			return nil, fault(Protocol)
		}
	}
	return vector, nil
}
