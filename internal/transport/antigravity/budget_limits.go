package antigravity

// InferenceLimits declares the fixed native request allowance. Butler reserves
// the entire context limit rather than guessing hidden provider tokenization.
// Credits/overage and alternate models remain unavailable in this adapter.
func (*Provider) InferenceLimits() (int64, int64) { return 1048576, maxOutputTokens }
