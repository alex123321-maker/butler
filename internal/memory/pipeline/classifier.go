package pipeline

import (
	"strings"

	"github.com/butler/butler/internal/memory/sanitize"
)

type CandidateKind string

const (
	CandidateProfile  CandidateKind = "profile"
	CandidateEpisode  CandidateKind = "episodic"
	CandidateWorking  CandidateKind = "working"
	CandidateDocument CandidateKind = "document"
	CandidateIgnore   CandidateKind = "ignore"
)

type ClassifiedProfile struct {
	Candidate ProfileCandidate
	ScopeType string
	ScopeID   string
	Reason    string
}

type ClassifiedEpisode struct {
	Candidate EpisodeCandidate
	ScopeType string
	ScopeID   string
	Reason    string
}

type ClassifiedWorking struct {
	Candidate WorkingCandidate
	ScopeType string
	ScopeID   string
	Reason    string
}

type ClassifiedDocument struct {
	Candidate DocumentCandidate
	ScopeType string
	ScopeID   string
	Reason    string
}

type IgnoredCandidate struct {
	Kind   CandidateKind
	Reason string
	Ref    string
}

type ClassificationResult struct {
	Profiles  []ClassifiedProfile
	Episodes  []ClassifiedEpisode
	Working   []ClassifiedWorking
	Documents []ClassifiedDocument
	Ignored   []IgnoredCandidate
	Summary   string
}

type Classifier struct{}

func NewClassifier() *Classifier { return &Classifier{} }

func (c *Classifier) Classify(sessionKey string, result *ExtractionResult) ClassificationResult {
	classified := ClassificationResult{Summary: sanitize.Text(strings.TrimSpace(result.SessionSummary))}
	if result == nil {
		return classified
	}
	for _, candidate := range result.ProfileUpdates {
		candidate = sanitizeProfileCandidate(candidate)
		scopeType, scopeID := classifyScope(candidate.ScopeType, candidate.ScopeID, sessionKey)
		if reason := ignoreProfileCandidate(candidate); reason != "" {
			classified.Ignored = append(classified.Ignored, IgnoredCandidate{Kind: CandidateProfile, Reason: reason, Ref: candidate.Key})
			continue
		}
		classified.Profiles = append(classified.Profiles, ClassifiedProfile{Candidate: candidate, ScopeType: scopeType, ScopeID: scopeID, Reason: "accepted_profile_candidate"})
	}
	for _, candidate := range result.Episodes {
		candidate = sanitizeEpisodeCandidate(candidate)
		scopeType, scopeID := classifyScope(candidate.ScopeType, candidate.ScopeID, sessionKey)
		if reason := ignoreEpisodeCandidate(candidate); reason != "" {
			classified.Ignored = append(classified.Ignored, IgnoredCandidate{Kind: CandidateEpisode, Reason: reason, Ref: candidate.Summary})
			continue
		}
		classified.Episodes = append(classified.Episodes, ClassifiedEpisode{Candidate: candidate, ScopeType: scopeType, ScopeID: scopeID, Reason: "accepted_episode_candidate"})
	}
	for _, candidate := range result.WorkingUpdates {
		candidate = sanitizeWorkingCandidate(candidate)
		scopeType, scopeID := classifyScope(candidate.ScopeType, candidate.ScopeID, sessionKey)
		if reason := ignoreWorkingCandidate(candidate); reason != "" {
			classified.Ignored = append(classified.Ignored, IgnoredCandidate{Kind: CandidateWorking, Reason: reason, Ref: candidate.Goal})
			continue
		}
		classified.Working = append(classified.Working, ClassifiedWorking{Candidate: candidate, ScopeType: scopeType, ScopeID: scopeID, Reason: "accepted_working_candidate"})
	}
	for _, candidate := range result.DocumentChunks {
		candidate = sanitizeDocumentCandidate(candidate)
		scopeType, scopeID := classifyScope(candidate.ScopeType, candidate.ScopeID, sessionKey)
		if reason := ignoreDocumentCandidate(candidate); reason != "" {
			classified.Ignored = append(classified.Ignored, IgnoredCandidate{Kind: CandidateDocument, Reason: reason, Ref: candidate.Title})
			continue
		}
		classified.Documents = append(classified.Documents, ClassifiedDocument{Candidate: candidate, ScopeType: scopeType, ScopeID: scopeID, Reason: "document_candidates_not_persisted_in_v1"})
		classified.Ignored = append(classified.Ignored, IgnoredCandidate{Kind: CandidateDocument, Reason: "document_persistence_not_implemented", Ref: candidate.Title})
		classified.Documents = classified.Documents[:len(classified.Documents)-1]
	}
	return classified
}

func classifyScope(scopeType, scopeID, sessionKey string) (string, string) {
	normalizedType := normalizeScopeType(scopeType)
	if strings.TrimSpace(scopeID) == "" {
		return normalizedType, scopeIDForType(normalizedType, sessionKey)
	}
	return normalizedType, strings.TrimSpace(scopeID)
}

func ignoreProfileCandidate(candidate ProfileCandidate) string {
	if candidate.Confidence < 0.5 {
		return "low_confidence"
	}
	if strings.TrimSpace(candidate.Key) == "" || strings.TrimSpace(candidate.Summary) == "" {
		return "missing_profile_fields"
	}
	if isNoiseText(candidate.Summary) {
		return "noise_summary"
	}
	return ""
}

func ignoreEpisodeCandidate(candidate EpisodeCandidate) string {
	if candidate.Confidence < 0.5 {
		return "low_confidence"
	}
	if strings.TrimSpace(candidate.Summary) == "" {
		return "missing_episode_summary"
	}
	if isNoiseText(candidate.Summary) && isNoiseText(candidate.Content) {
		return "noise_episode"
	}
	return ""
}

func ignoreWorkingCandidate(candidate WorkingCandidate) string {
	if candidate.Confidence < 0.5 {
		return "low_confidence"
	}
	if strings.TrimSpace(candidate.Goal) == "" && strings.TrimSpace(candidate.Summary) == "" {
		return "missing_working_state"
	}
	return ""
}

func ignoreDocumentCandidate(candidate DocumentCandidate) string {
	if candidate.Confidence < 0.7 {
		return "low_confidence"
	}
	if strings.TrimSpace(candidate.Title) == "" || strings.TrimSpace(candidate.Content) == "" {
		return "missing_document_fields"
	}
	return ""
}

func isNoiseText(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	if trimmed == "" {
		return true
	}
	for _, noise := range []string{"ok", "thanks", "thank you", "noted", "done", "hello", "hi"} {
		if trimmed == noise {
			return true
		}
	}
	return false
}

func sanitizeProfileCandidate(candidate ProfileCandidate) ProfileCandidate {
	candidate.Key = sanitize.Text(candidate.Key)
	candidate.Value = sanitize.JSON(candidate.Value)
	candidate.Summary = sanitize.Text(candidate.Summary)
	return candidate
}

func sanitizeEpisodeCandidate(candidate EpisodeCandidate) EpisodeCandidate {
	candidate.Summary = sanitize.Text(candidate.Summary)
	candidate.Content = sanitize.Text(candidate.Content)
	return candidate
}

func sanitizeWorkingCandidate(candidate WorkingCandidate) WorkingCandidate {
	candidate.Goal = sanitize.Text(candidate.Goal)
	candidate.Summary = sanitize.Text(candidate.Summary)
	return candidate
}

func sanitizeDocumentCandidate(candidate DocumentCandidate) DocumentCandidate {
	candidate.Title = sanitize.Text(candidate.Title)
	candidate.Content = sanitize.Text(candidate.Content)
	return candidate
}

type ConflictResolver struct{}

func NewConflictResolver() *ConflictResolver { return &ConflictResolver{} }

type ResolvedProfile struct {
	ClassifiedProfile
	Action string
}

type ResolvedEpisode struct {
	ClassifiedEpisode
	Action string
}

type ResolutionResult struct {
	Profiles []ResolvedProfile
	Episodes []ResolvedEpisode
	Working  []ClassifiedWorking
	Ignored  []IgnoredCandidate
	Summary  string
}

func (r *ConflictResolver) Resolve(classified ClassificationResult) ResolutionResult {
	resolved := ResolutionResult{Summary: classified.Summary, Working: classified.Working, Ignored: append([]IgnoredCandidate(nil), classified.Ignored...)}
	profileSeen := map[string]ResolvedProfile{}
	for _, candidate := range classified.Profiles {
		key := candidate.ScopeType + ":" + candidate.ScopeID + ":" + candidate.Candidate.Key
		existing, ok := profileSeen[key]
		if !ok || candidate.Candidate.Confidence >= existing.Candidate.Confidence {
			if ok {
				resolved.Ignored = append(resolved.Ignored, IgnoredCandidate{Kind: CandidateProfile, Reason: "duplicate_profile_replaced", Ref: existing.Candidate.Key})
			}
			profileSeen[key] = ResolvedProfile{ClassifiedProfile: candidate, Action: "upsert_profile"}
		} else {
			resolved.Ignored = append(resolved.Ignored, IgnoredCandidate{Kind: CandidateProfile, Reason: "duplicate_profile_lower_confidence", Ref: candidate.Candidate.Key})
		}
	}
	for _, item := range profileSeen {
		resolved.Profiles = append(resolved.Profiles, item)
	}
	episodeSeen := map[string]ResolvedEpisode{}
	for _, candidate := range classified.Episodes {
		key := candidate.ScopeType + ":" + candidate.ScopeID + ":" + strings.ToLower(strings.TrimSpace(candidate.Candidate.Summary))
		existing, ok := episodeSeen[key]
		if !ok || candidate.Candidate.Confidence >= existing.Candidate.Confidence {
			if ok {
				resolved.Ignored = append(resolved.Ignored, IgnoredCandidate{Kind: CandidateEpisode, Reason: "duplicate_episode_replaced", Ref: existing.Candidate.Summary})
			}
			episodeSeen[key] = ResolvedEpisode{ClassifiedEpisode: candidate, Action: "create_episode"}
		} else {
			resolved.Ignored = append(resolved.Ignored, IgnoredCandidate{Kind: CandidateEpisode, Reason: "duplicate_episode_lower_confidence", Ref: candidate.Candidate.Summary})
		}
	}
	for _, item := range episodeSeen {
		resolved.Episodes = append(resolved.Episodes, item)
	}
	if len(classified.Documents) > 0 {
		for _, doc := range classified.Documents {
			resolved.Ignored = append(resolved.Ignored, IgnoredCandidate{Kind: CandidateDocument, Reason: "document_persistence_not_implemented", Ref: doc.Candidate.Title})
		}
	}
	return resolved
}
