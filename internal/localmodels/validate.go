package localmodels

import (
	"strings"
	"unicode/utf8"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

func validateRequest(p *pb.ModelProfile, r *pb.InferRequest) error {
	if r.RequestId == "" || len(r.RequestId) > 128 || !utf8.ValidString(r.RequestId) || r.ProfileId == "" || len(r.CallerId) > 128 || !utf8.ValidString(r.CallerId) {
		return fault(Invalid)
	}
	cap := pb.Capability_CAPABILITY_UNSPECIFIED
	count, bytes := 0, 0
	validText := func(s string) bool { return utf8.ValidString(s) && strings.TrimSpace(s) != "" }
	switch input := r.Input.(type) {
	case *pb.InferRequest_Embed:
		cap = pb.Capability_CAPABILITY_EMBED
		if input.Embed == nil || len(input.Embed.Texts) == 0 || input.Embed.Task < pb.EmbeddingTask_EMBEDDING_TASK_QUERY || input.Embed.Task > pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY {
			return fault(Invalid)
		}
		if input.Embed.Dimensions != 256 && input.Embed.Dimensions != 768 || input.Embed.Dimensions > p.Dimensions {
			return fault(Invalid)
		}
		seen := map[string]bool{}
		for _, t := range input.Embed.Texts {
			if t == nil || t.Id == "" || len(t.Id) > 128 || !utf8.ValidString(t.Id) || seen[t.Id] || !validText(t.Text) || !utf8.ValidString(t.Title) {
				return fault(Invalid)
			}
			seen[t.Id] = true
			count++
			bytes += len(t.Text) + len(t.Title)
		}
	case *pb.InferRequest_Rerank:
		cap = pb.Capability_CAPABILITY_RERANK
		if input.Rerank == nil || !validText(input.Rerank.Query) {
			return fault(Invalid)
		}
		bytes = len(input.Rerank.Query)
		seen := map[string]bool{}
		for _, t := range input.Rerank.Candidates {
			if t == nil || t.Id == "" || len(t.Id) > 128 || !utf8.ValidString(t.Id) || seen[t.Id] || !validText(t.Text) || !utf8.ValidString(t.Title) {
				return fault(Invalid)
			}
			seen[t.Id] = true
			count++
			bytes += len(t.Text) + len(t.Title)
		}
	case *pb.InferRequest_Extract:
		cap = pb.Capability_CAPABILITY_EXTRACT
		if input.Extract == nil || !validText(input.Extract.Text) || input.Extract.SchemaVersion == "" || len(input.Extract.EntityTypes) == 0 {
			return fault(Invalid)
		}
		count = 1
		bytes = len(input.Extract.Text)
		for _, label := range input.Extract.EntityTypes {
			if !validText(label) {
				return fault(Invalid)
			}
			bytes += len(label)
		}
	case *pb.InferRequest_Classify:
		cap = pb.Capability_CAPABILITY_CLASSIFY
		if input.Classify == nil || !validText(input.Classify.Text) || len(input.Classify.Labels) < 2 {
			return fault(Invalid)
		}
		count = 1
		bytes = len(input.Classify.Text)
		for _, label := range input.Classify.Labels {
			if !validText(label) {
				return fault(Invalid)
			}
			bytes += len(label)
		}
	case *pb.InferRequest_Media:
		cap = pb.Capability_CAPABILITY_MEDIA
		if input.Media == nil || len(input.Media.Content) == 0 || input.Media.MimeType == "" {
			return fault(Invalid)
		}
		count = 1
		bytes = len(input.Media.Content)
	default:
		return fault(Invalid)
	}
	supported := false
	for _, c := range p.Capabilities {
		if c == cap {
			supported = true
		}
	}
	if !supported {
		return fault(Unavailable)
	}
	if count == 0 || count > int(p.MaxBatch) || bytes > int(p.MaxInputBytes) {
		return fault(Invalid)
	}
	return nil
}
