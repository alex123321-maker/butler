package localmodels

import pb "github.com/butler/butler/internal/gen/localmodels/v1"

// GLiNERFullScores is an opt-in preprocessing identity. Existing selected-only
// profiles retain their original output and scores for reproducible comparison.
const GLiNERFullScores = "gliner2.5-classification-softmax-v1"

func fullScoreProfile(p *pb.ModelProfile) bool {
	return p.PreprocessingVersion == GLiNERFullScores &&
		(p.Model == "fastino/gliner2.5-multi-v1" || p.Model == "fastino/GLiNER2.5-multi-Decide")
}
