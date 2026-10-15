package compaction

// ReleaseQualified is code-owned, not an environment/owner override. Controlled
// boundary tests and a partial native window cannot qualify production use.
// The paired ordinary-task native gate must pass before this is changed.
const ReleaseQualified = false
